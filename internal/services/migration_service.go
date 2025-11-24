package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/migration/schemas"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
)

const (
	PrefRioServicesCollection  = "prefrio_services_base"
	MigrationControlCollection = "_migration_control"
	BackupCollectionPrefix     = "prefrio_services_backup_"
)

// MigrationService gerencia migrações de schema
type MigrationService struct {
	client         *typesense.Client
	schemaRegistry *schemas.Registry
}

// NewMigrationService cria um novo serviço de migração
func NewMigrationService(client *typesense.Client, registry *schemas.Registry) *MigrationService {
	return &MigrationService{
		client:         client,
		schemaRegistry: registry,
	}
}

// GetStatus retorna o status atual da migração
func (ms *MigrationService) GetStatus(ctx context.Context) (*models.MigrationStatusResponse, error) {
	migration, err := ms.getActiveMigration(ctx)
	if err != nil {
		return nil, err
	}

	if migration == nil {
		return &models.MigrationStatusResponse{
			Status:   models.MigrationStatusIdle,
			IsLocked: false,
		}, nil
	}

	progress := float64(0)
	if migration.TotalDocuments > 0 {
		progress = float64(migration.MigratedDocuments) / float64(migration.TotalDocuments) * 100
	}

	return &models.MigrationStatusResponse{
		Status:            migration.Status,
		SchemaVersion:     migration.SchemaVersion,
		SourceCollection:  migration.SourceCollection,
		TargetCollection:  migration.TargetCollection,
		BackupCollection:  migration.BackupCollection,
		StartedAt:         migration.StartedAt,
		CompletedAt:       migration.CompletedAt,
		StartedBy:         migration.StartedBy,
		TotalDocuments:    migration.TotalDocuments,
		MigratedDocuments: migration.MigratedDocuments,
		Progress:          progress,
		ErrorMessage:      migration.ErrorMessage,
		IsLocked:          migration.IsLocked,
	}, nil
}

// StartMigration inicia o processo de migração para uma nova versão de schema
func (ms *MigrationService) StartMigration(ctx context.Context, req *models.MigrationStartRequest, userName, userCPF string) (*models.MigrationStatusResponse, error) {
	active, err := ms.getActiveMigration(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao verificar migração ativa: %v", err)
	}
	if active != nil {
		return nil, fmt.Errorf("já existe uma migração em andamento (ID: %s)", active.ID)
	}

	schema, err := ms.schemaRegistry.GetSchema(req.SchemaVersion)
	if err != nil {
		return nil, fmt.Errorf("schema versão '%s' não encontrado: %v", req.SchemaVersion, err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupCollectionName := fmt.Sprintf("%s%s", BackupCollectionPrefix, timestamp)
	targetCollectionName := fmt.Sprintf("%s_v%s_%s", PrefRioServicesCollection, req.SchemaVersion, timestamp)

	totalDocs, err := ms.countDocuments(ctx, PrefRioServicesCollection)
	if err != nil {
		return nil, fmt.Errorf("erro ao contar documentos: %v", err)
	}

	previousVersion := ms.schemaRegistry.GetCurrentVersion()

	migration := &models.MigrationControl{
		Status:                models.MigrationStatusInProgress,
		SourceCollection:      PrefRioServicesCollection,
		TargetCollection:      targetCollectionName,
		BackupCollection:      backupCollectionName,
		SchemaVersion:         req.SchemaVersion,
		PreviousSchemaVersion: previousVersion,
		StartedAt:             time.Now().Unix(),
		StartedBy:             userName,
		StartedByCPF:          userCPF,
		TotalDocuments:        totalDocs,
		MigratedDocuments:     0,
		IsLocked:              true,
	}

	createdMigration, err := ms.createMigrationControl(ctx, migration)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar registro de migração: %v", err)
	}

	if req.DryRun {
		createdMigration.Status = models.MigrationStatusCompleted
		createdMigration.IsLocked = false
		createdMigration.CompletedAt = time.Now().Unix()
		ms.updateMigrationControl(ctx, createdMigration.ID, createdMigration)
		
		return &models.MigrationStatusResponse{
			Status:           models.MigrationStatusCompleted,
			SchemaVersion:    req.SchemaVersion,
			SourceCollection: PrefRioServicesCollection,
			TargetCollection: targetCollectionName,
			BackupCollection: backupCollectionName,
			TotalDocuments:   totalDocs,
			IsLocked:         false,
		}, nil
	}

	go ms.executeMigration(context.Background(), createdMigration, schema)

	return &models.MigrationStatusResponse{
		Status:            models.MigrationStatusInProgress,
		SchemaVersion:     req.SchemaVersion,
		SourceCollection:  PrefRioServicesCollection,
		TargetCollection:  targetCollectionName,
		BackupCollection:  backupCollectionName,
		StartedAt:         createdMigration.StartedAt,
		StartedBy:         userName,
		TotalDocuments:    totalDocs,
		MigratedDocuments: 0,
		Progress:          0,
		IsLocked:          true,
	}, nil
}

// executeMigration executa o processo completo de migração em background
func (ms *MigrationService) executeMigration(ctx context.Context, migration *models.MigrationControl, schema *schemas.SchemaDefinition) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERRO: Panic durante migração: %v", r)
			migration.Status = models.MigrationStatusFailed
			migration.ErrorMessage = fmt.Sprintf("panic: %v", r)
			migration.IsLocked = false
			ms.updateMigrationControl(ctx, migration.ID, migration)
		}
	}()

	log.Printf("[Migration] Iniciando migração para schema %s", migration.SchemaVersion)

	if err := ms.createBackup(ctx, migration); err != nil {
		ms.failMigration(ctx, migration, fmt.Sprintf("erro ao criar backup: %v", err))
		return
	}
	log.Printf("[Migration] Backup criado: %s", migration.BackupCollection)

	if err := ms.createNewCollection(ctx, migration, schema); err != nil {
		ms.failMigration(ctx, migration, fmt.Sprintf("erro ao criar collection: %v", err))
		return
	}
	log.Printf("[Migration] Nova collection criada: %s", migration.TargetCollection)

	if err := ms.migrateDocuments(ctx, migration, schema); err != nil {
		ms.failMigration(ctx, migration, fmt.Sprintf("erro ao migrar documentos: %v", err))
		return
	}
	log.Printf("[Migration] Documentos migrados: %d", migration.MigratedDocuments)

	if err := ms.validateMigration(ctx, migration); err != nil {
		ms.failMigration(ctx, migration, fmt.Sprintf("validação falhou: %v", err))
		return
	}
	log.Printf("[Migration] Validação concluída")

	if err := ms.swapCollections(ctx, migration); err != nil {
		ms.failMigration(ctx, migration, fmt.Sprintf("erro ao trocar collections: %v", err))
		return
	}
	log.Printf("[Migration] Collections trocadas com sucesso")

	ms.completeMigration(ctx, migration)
	log.Printf("[Migration] Migração concluída com sucesso!")
}

// createBackup cria uma cópia completa da collection atual
func (ms *MigrationService) createBackup(ctx context.Context, migration *models.MigrationControl) error {
	sourceSchema, err := ms.client.Collection(migration.SourceCollection).Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("erro ao obter schema da collection origem: %v", err)
	}

	backupSchema := &api.CollectionSchema{
		Name:                migration.BackupCollection,
		Fields:              sourceSchema.Fields,
		DefaultSortingField: sourceSchema.DefaultSortingField,
		EnableNestedFields:  sourceSchema.EnableNestedFields,
	}

	_, err = ms.client.Collections().Create(ctx, backupSchema)
	if err != nil {
		return fmt.Errorf("erro ao criar collection de backup: %v", err)
	}

	page := 1
	perPage := 250
	totalCopied := 0

	for {
		docs, err := ms.fetchDocuments(ctx, migration.SourceCollection, page, perPage)
		if err != nil {
			return fmt.Errorf("erro ao buscar documentos (página %d): %v", page, err)
		}

		if len(docs) == 0 {
			break
		}

		if err := ms.importDocuments(ctx, migration.BackupCollection, docs); err != nil {
			return fmt.Errorf("erro ao importar documentos no backup (página %d): %v", page, err)
		}

		totalCopied += len(docs)

		if len(docs) < perPage {
			break
		}

		page++
	}

	log.Printf("[Migration] Backup: %d documentos copiados para %s", totalCopied, migration.BackupCollection)
	return nil
}

// createNewCollection cria a nova collection com o novo schema
func (ms *MigrationService) createNewCollection(ctx context.Context, migration *models.MigrationControl, schema *schemas.SchemaDefinition) error {
	newSchema := &api.CollectionSchema{
		Name:                migration.TargetCollection,
		Fields:              schema.Fields,
		DefaultSortingField: stringPtr(schema.SortingField),
		EnableNestedFields:  boolPtr(schema.NestedFields),
	}

	_, err := ms.client.Collections().Create(ctx, newSchema)
	if err != nil {
		return fmt.Errorf("erro ao criar nova collection: %v", err)
	}

	return nil
}

// migrateDocuments migra todos os documentos aplicando transformações se necessário
func (ms *MigrationService) migrateDocuments(ctx context.Context, migration *models.MigrationControl, schema *schemas.SchemaDefinition) error {
	page := 1
	perPage := 250
	totalMigrated := 0

	for {
		docs, err := ms.fetchDocuments(ctx, migration.SourceCollection, page, perPage)
		if err != nil {
			return fmt.Errorf("erro ao buscar documentos (página %d): %v", page, err)
		}

		if len(docs) == 0 {
			break
		}

		transformedDocs := make([]map[string]interface{}, 0, len(docs))
		for _, doc := range docs {
			var transformed map[string]interface{}
			if schema.Transform != nil {
				transformed, err = schema.Transform(doc)
				if err != nil {
					return fmt.Errorf("erro ao transformar documento: %v", err)
				}
			} else {
				transformed = doc
			}
			transformedDocs = append(transformedDocs, transformed)
		}

		if err := ms.importDocuments(ctx, migration.TargetCollection, transformedDocs); err != nil {
			return fmt.Errorf("erro ao importar documentos (página %d): %v", page, err)
		}

		totalMigrated += len(docs)
		migration.MigratedDocuments = totalMigrated
		ms.updateMigrationControl(ctx, migration.ID, migration)

		if len(docs) < perPage {
			break
		}

		page++
	}

	return nil
}

// validateMigration valida que a migração foi bem-sucedida
func (ms *MigrationService) validateMigration(ctx context.Context, migration *models.MigrationControl) error {
	sourceCount, err := ms.countDocuments(ctx, migration.SourceCollection)
	if err != nil {
		return fmt.Errorf("erro ao contar documentos na origem: %v", err)
	}

	targetCount, err := ms.countDocuments(ctx, migration.TargetCollection)
	if err != nil {
		return fmt.Errorf("erro ao contar documentos no destino: %v", err)
	}

	if sourceCount != targetCount {
		return fmt.Errorf("contagem de documentos difere: origem=%d, destino=%d", sourceCount, targetCount)
	}

	log.Printf("[Migration] Validação: %d documentos em ambas collections", sourceCount)
	return nil
}

// swapCollections troca as collections (renomeia a antiga e a nova)
func (ms *MigrationService) swapCollections(ctx context.Context, migration *models.MigrationControl) error {
	oldCollectionName := fmt.Sprintf("%s_old_%d", migration.SourceCollection, time.Now().Unix())

	_, err := ms.client.Collection(migration.SourceCollection).Retrieve(ctx)
	if err == nil {
		aliasSchema := &api.CollectionAliasSchema{
			CollectionName: oldCollectionName,
		}
		
		existingDocs, _ := ms.fetchDocuments(ctx, migration.SourceCollection, 1, 1)
		if len(existingDocs) > 0 {
			sourceSchema, err := ms.client.Collection(migration.SourceCollection).Retrieve(ctx)
			if err != nil {
				return fmt.Errorf("erro ao obter schema da collection origem: %v", err)
			}

			oldSchema := &api.CollectionSchema{
				Name:                oldCollectionName,
				Fields:              sourceSchema.Fields,
				DefaultSortingField: sourceSchema.DefaultSortingField,
				EnableNestedFields:  sourceSchema.EnableNestedFields,
			}
			_, err = ms.client.Collections().Create(ctx, oldSchema)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("erro ao criar collection antiga: %v", err)
			}

			allDocs := []map[string]interface{}{}
			page := 1
			for {
				docs, err := ms.fetchDocuments(ctx, migration.SourceCollection, page, 250)
				if err != nil {
					return fmt.Errorf("erro ao buscar documentos para mover: %v", err)
				}
				if len(docs) == 0 {
					break
				}
				allDocs = append(allDocs, docs...)
				if len(docs) < 250 {
					break
				}
				page++
			}

			if err := ms.importDocuments(ctx, oldCollectionName, allDocs); err != nil {
				return fmt.Errorf("erro ao copiar documentos para collection antiga: %v", err)
			}
		}

		_, err = ms.client.Aliases().Upsert(ctx, migration.SourceCollection, aliasSchema)
		if err != nil {
			log.Printf("[Migration] Aviso: não foi possível criar alias, deletando collection antiga: %v", err)
			_, delErr := ms.client.Collection(migration.SourceCollection).Delete(ctx)
			if delErr != nil {
				return fmt.Errorf("erro ao deletar collection original: %v", delErr)
			}
		}
	}

	aliasSchema := &api.CollectionAliasSchema{
		CollectionName: migration.TargetCollection,
	}
	_, err = ms.client.Aliases().Upsert(ctx, PrefRioServicesCollection, aliasSchema)
	if err != nil {
		return fmt.Errorf("erro ao criar alias para nova collection: %v", err)
	}

	log.Printf("[Migration] Alias %s agora aponta para %s", PrefRioServicesCollection, migration.TargetCollection)
	return nil
}

// completeMigration finaliza a migração com sucesso
func (ms *MigrationService) completeMigration(ctx context.Context, migration *models.MigrationControl) {
	migration.Status = models.MigrationStatusCompleted
	migration.CompletedAt = time.Now().Unix()
	migration.IsLocked = false
	ms.updateMigrationControl(ctx, migration.ID, migration)
}

// failMigration marca a migração como falha
func (ms *MigrationService) failMigration(ctx context.Context, migration *models.MigrationControl, errorMsg string) {
	log.Printf("[Migration] FALHA: %s", errorMsg)
	migration.Status = models.MigrationStatusFailed
	migration.ErrorMessage = errorMsg
	migration.IsLocked = false
	ms.updateMigrationControl(ctx, migration.ID, migration)
}

// RollbackMigration executa rollback para a versão anterior
func (ms *MigrationService) RollbackMigration(ctx context.Context, req *models.MigrationRollbackRequest, userName, userCPF string) (*models.MigrationStatusResponse, error) {
	var migrationToRollback *models.MigrationControl
	var err error

	if req.MigrationID != "" {
		migrationToRollback, err = ms.getMigrationControl(ctx, req.MigrationID)
		if err != nil {
			return nil, fmt.Errorf("migração não encontrada: %v", err)
		}
	} else {
		migrationToRollback, err = ms.getLatestCompletedMigration(ctx)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar última migração: %v", err)
		}
		if migrationToRollback == nil {
			return nil, fmt.Errorf("nenhuma migração encontrada para rollback")
		}
	}

	if migrationToRollback.BackupCollection == "" {
		return nil, fmt.Errorf("migração não possui collection de backup")
	}

	_, err = ms.client.Collection(migrationToRollback.BackupCollection).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("collection de backup não encontrada: %s", migrationToRollback.BackupCollection)
	}

	active, _ := ms.getActiveMigration(ctx)
	if active != nil {
		return nil, fmt.Errorf("existe uma migração em andamento, aguarde sua conclusão")
	}

	rollbackMigration := &models.MigrationControl{
		Status:                models.MigrationStatusRollback,
		SourceCollection:      migrationToRollback.TargetCollection,
		TargetCollection:      migrationToRollback.BackupCollection,
		BackupCollection:      "",
		SchemaVersion:         migrationToRollback.PreviousSchemaVersion,
		PreviousSchemaVersion: migrationToRollback.SchemaVersion,
		StartedAt:             time.Now().Unix(),
		StartedBy:             userName,
		StartedByCPF:          userCPF,
		TotalDocuments:        migrationToRollback.TotalDocuments,
		MigratedDocuments:     0,
		IsLocked:              true,
	}

	createdRollback, err := ms.createMigrationControl(ctx, rollbackMigration)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar registro de rollback: %v", err)
	}

	aliasSchema := &api.CollectionAliasSchema{
		CollectionName: migrationToRollback.BackupCollection,
	}
	_, err = ms.client.Aliases().Upsert(ctx, PrefRioServicesCollection, aliasSchema)
	if err != nil {
		createdRollback.Status = models.MigrationStatusFailed
		createdRollback.ErrorMessage = fmt.Sprintf("erro ao restaurar alias: %v", err)
		createdRollback.IsLocked = false
		ms.updateMigrationControl(ctx, createdRollback.ID, createdRollback)
		return nil, fmt.Errorf("erro ao restaurar alias: %v", err)
	}

	backupCount, _ := ms.countDocuments(ctx, migrationToRollback.BackupCollection)

	createdRollback.Status = models.MigrationStatusCompleted
	createdRollback.MigratedDocuments = backupCount
	createdRollback.CompletedAt = time.Now().Unix()
	createdRollback.IsLocked = false
	ms.updateMigrationControl(ctx, createdRollback.ID, createdRollback)

	log.Printf("[Migration] Rollback concluído: alias %s agora aponta para %s",
		PrefRioServicesCollection, migrationToRollback.BackupCollection)

	return &models.MigrationStatusResponse{
		Status:            models.MigrationStatusCompleted,
		SchemaVersion:     migrationToRollback.PreviousSchemaVersion,
		SourceCollection:  migrationToRollback.TargetCollection,
		TargetCollection:  migrationToRollback.BackupCollection,
		TotalDocuments:    backupCount,
		MigratedDocuments: backupCount,
		Progress:          100,
		IsLocked:          false,
	}, nil
}

// GetHistory retorna o histórico de migrações
func (ms *MigrationService) GetHistory(ctx context.Context, page, perPage int) (*models.MigrationHistoryResponse, error) {
	return ms.listMigrationHistory(ctx, page, perPage)
}

// IsMigrationLocked verifica se o sistema está bloqueado por uma migração
func (ms *MigrationService) IsMigrationLocked(ctx context.Context) (bool, error) {
	migration, err := ms.getActiveMigration(ctx)
	if err != nil {
		return false, err
	}

	if migration != nil && migration.IsLocked {
		return true, nil
	}

	return false, nil
}

// ========== Métodos auxiliares ==========

func (ms *MigrationService) countDocuments(ctx context.Context, collection string) (int, error) {
	searchParams := &api.SearchCollectionParams{
		Q:       stringPtr("*"),
		Page:    intPtr(1),
		PerPage: intPtr(0),
	}

	result, err := ms.client.Collection(collection).Documents().Search(ctx, searchParams)
	if err != nil {
		return 0, err
	}

	if result.Found != nil {
		return int(*result.Found), nil
	}

	return 0, nil
}

func (ms *MigrationService) fetchDocuments(ctx context.Context, collection string, page, perPage int) ([]map[string]interface{}, error) {
	searchParams := &api.SearchCollectionParams{
		Q:       stringPtr("*"),
		Page:    intPtr(page),
		PerPage: intPtr(perPage),
	}

	result, err := ms.client.Collection(collection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, err
	}

	var docs []map[string]interface{}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &resultMap); err != nil {
		return nil, err
	}

	if hits, ok := resultMap["hits"].([]interface{}); ok {
		for _, hit := range hits {
			if hitMap, ok := hit.(map[string]interface{}); ok {
				if doc, ok := hitMap["document"].(map[string]interface{}); ok {
					docs = append(docs, doc)
				}
			}
		}
	}

	return docs, nil
}

func (ms *MigrationService) importDocuments(ctx context.Context, collection string, docs []map[string]interface{}) error {
	if len(docs) == 0 {
		return nil
	}

	for _, doc := range docs {
		_, err := ms.client.Collection(collection).Documents().Create(ctx, doc, &api.DocumentIndexParameters{})
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return fmt.Errorf("erro ao importar documento: %v", err)
		}
	}

	return nil
}

// Métodos de acesso à collection _migration_control
func (ms *MigrationService) ensureMigrationControlCollection(ctx context.Context) error {
	_, err := ms.client.Collection(MigrationControlCollection).Retrieve(ctx)
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not found") {
		schema := &api.CollectionSchema{
			Name: MigrationControlCollection,
			Fields: []api.Field{
				{Name: "id", Type: "string", Optional: boolPtr(true)},
				{Name: "status", Type: "string", Facet: boolPtr(true)},
				{Name: "source_collection", Type: "string", Facet: boolPtr(false)},
				{Name: "target_collection", Type: "string", Facet: boolPtr(false)},
				{Name: "backup_collection", Type: "string", Facet: boolPtr(false)},
				{Name: "schema_version", Type: "string", Facet: boolPtr(true)},
				{Name: "previous_schema_version", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
				{Name: "started_at", Type: "int64", Facet: boolPtr(false)},
				{Name: "completed_at", Type: "int64", Facet: boolPtr(false), Optional: boolPtr(true)},
				{Name: "started_by", Type: "string", Facet: boolPtr(true)},
				{Name: "started_by_cpf", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
				{Name: "total_documents", Type: "int32", Facet: boolPtr(false)},
				{Name: "migrated_documents", Type: "int32", Facet: boolPtr(false)},
				{Name: "error_message", Type: "string", Facet: boolPtr(false), Optional: boolPtr(true)},
				{Name: "is_locked", Type: "bool", Facet: boolPtr(true)},
			},
			DefaultSortingField: stringPtr("started_at"),
		}

		_, err = ms.client.Collections().Create(ctx, schema)
		if err != nil {
			return fmt.Errorf("erro ao criar collection %s: %v", MigrationControlCollection, err)
		}
	}

	return err
}

func (ms *MigrationService) createMigrationControl(ctx context.Context, migration *models.MigrationControl) (*models.MigrationControl, error) {
	if err := ms.ensureMigrationControlCollection(ctx); err != nil {
		return nil, err
	}

	migrationMap := structToMapMigration(migration)
	if migration.ID == "" {
		delete(migrationMap, "id")
	}

	result, err := ms.client.Collection(MigrationControlCollection).Documents().Create(ctx, migrationMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, err
	}

	resultBytes, _ := json.Marshal(result)
	var created models.MigrationControl
	json.Unmarshal(resultBytes, &created)

	return &created, nil
}

func (ms *MigrationService) updateMigrationControl(ctx context.Context, id string, migration *models.MigrationControl) (*models.MigrationControl, error) {
	migration.ID = id
	migrationMap := structToMapMigration(migration)

	result, err := ms.client.Collection(MigrationControlCollection).Document(id).Update(ctx, migrationMap, &api.DocumentIndexParameters{})
	if err != nil {
		return nil, err
	}

	resultBytes, _ := json.Marshal(result)
	var updated models.MigrationControl
	json.Unmarshal(resultBytes, &updated)

	return &updated, nil
}

func (ms *MigrationService) getMigrationControl(ctx context.Context, id string) (*models.MigrationControl, error) {
	if err := ms.ensureMigrationControlCollection(ctx); err != nil {
		return nil, err
	}

	result, err := ms.client.Collection(MigrationControlCollection).Document(id).Retrieve(ctx)
	if err != nil {
		return nil, err
	}

	resultBytes, _ := json.Marshal(result)
	var migration models.MigrationControl
	json.Unmarshal(resultBytes, &migration)

	return &migration, nil
}

func (ms *MigrationService) getActiveMigration(ctx context.Context) (*models.MigrationControl, error) {
	if err := ms.ensureMigrationControlCollection(ctx); err != nil {
		return nil, err
	}

	filterBy := "status:=in_progress"
	searchParams := &api.SearchCollectionParams{
		Q:        stringPtr("*"),
		FilterBy: &filterBy,
		Page:     intPtr(1),
		PerPage:  intPtr(1),
		SortBy:   stringPtr("started_at:desc"),
	}

	result, err := ms.client.Collection(MigrationControlCollection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, err
	}

	jsonData, _ := json.Marshal(result)
	var resultMap map[string]interface{}
	json.Unmarshal(jsonData, &resultMap)

	if found, ok := resultMap["found"].(float64); ok && found > 0 {
		if hits, ok := resultMap["hits"].([]interface{}); ok && len(hits) > 0 {
			if hitMap, ok := hits[0].(map[string]interface{}); ok {
				if doc, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(doc)
					var migration models.MigrationControl
					json.Unmarshal(docBytes, &migration)
					return &migration, nil
				}
			}
		}
	}

	return nil, nil
}

func (ms *MigrationService) getLatestCompletedMigration(ctx context.Context) (*models.MigrationControl, error) {
	if err := ms.ensureMigrationControlCollection(ctx); err != nil {
		return nil, err
	}

	filterBy := "status:=completed"
	searchParams := &api.SearchCollectionParams{
		Q:        stringPtr("*"),
		FilterBy: &filterBy,
		Page:     intPtr(1),
		PerPage:  intPtr(1),
		SortBy:   stringPtr("completed_at:desc"),
	}

	result, err := ms.client.Collection(MigrationControlCollection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, err
	}

	jsonData, _ := json.Marshal(result)
	var resultMap map[string]interface{}
	json.Unmarshal(jsonData, &resultMap)

	if found, ok := resultMap["found"].(float64); ok && found > 0 {
		if hits, ok := resultMap["hits"].([]interface{}); ok && len(hits) > 0 {
			if hitMap, ok := hits[0].(map[string]interface{}); ok {
				if doc, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(doc)
					var migration models.MigrationControl
					json.Unmarshal(docBytes, &migration)
					return &migration, nil
				}
			}
		}
	}

	return nil, nil
}

func (ms *MigrationService) listMigrationHistory(ctx context.Context, page, perPage int) (*models.MigrationHistoryResponse, error) {
	if err := ms.ensureMigrationControlCollection(ctx); err != nil {
		return nil, err
	}

	searchParams := &api.SearchCollectionParams{
		Q:       stringPtr("*"),
		Page:    intPtr(page),
		PerPage: intPtr(perPage),
		SortBy:  stringPtr("started_at:desc"),
	}

	result, err := ms.client.Collection(MigrationControlCollection).Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, err
	}

	jsonData, _ := json.Marshal(result)
	var resultMap map[string]interface{}
	json.Unmarshal(jsonData, &resultMap)

	var migrations []models.MigrationHistoryItem
	if hits, ok := resultMap["hits"].([]interface{}); ok {
		for _, hit := range hits {
			if hitMap, ok := hit.(map[string]interface{}); ok {
				if doc, ok := hitMap["document"].(map[string]interface{}); ok {
					docBytes, _ := json.Marshal(doc)
					var item models.MigrationHistoryItem
					json.Unmarshal(docBytes, &item)
					migrations = append(migrations, item)
				}
			}
		}
	}

	found := 0
	if foundFloat, ok := resultMap["found"].(float64); ok {
		found = int(foundFloat)
	}

	return &models.MigrationHistoryResponse{
		Found:      found,
		OutOf:      found,
		Page:       page,
		Migrations: migrations,
	}, nil
}

// structToMapMigration converte uma struct para map[string]interface{} para uso interno da migração
func structToMapMigration(v interface{}) map[string]interface{} {
	data, _ := json.Marshal(v)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}

