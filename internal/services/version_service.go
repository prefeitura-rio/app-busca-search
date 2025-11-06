package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/typesense/typesense-go/v3/typesense"
	api "github.com/typesense/typesense-go/v3/typesense/api"
	"github.com/typesense/typesense-go/v3/typesense/api/pointer"
)

// VersionService gerencia o histórico de versões dos serviços
type VersionService struct {
	typesenseClient *typesense.Client
}

// NewVersionService cria uma nova instância do VersionService
func NewVersionService(typesenseClient *typesense.Client) *VersionService {
	return &VersionService{
		typesenseClient: typesenseClient,
	}
}

// CaptureVersion captura uma nova versão do serviço
func (vs *VersionService) CaptureVersion(
	ctx context.Context,
	service *models.PrefRioService,
	changeType string,
	createdBy string,
	createdByCPF string,
	changeReason string,
	previousVersion *models.ServiceVersion,
) (*models.ServiceVersion, error) {
	log.Printf("[CaptureVersion] Iniciando para serviceID=%s, changeType=%s, createdBy='%s', createdByCPF='%s'",
		service.ID, changeType, createdBy, createdByCPF)

	// Determina o número da versão
	versionNumber := int64(1)
	var previousVersionNumber int64
	if previousVersion != nil {
		versionNumber = previousVersion.VersionNumber + 1
		previousVersionNumber = previousVersion.VersionNumber
		log.Printf("[CaptureVersion] Versão anterior encontrada: %d, nova versão será: %d", previousVersionNumber, versionNumber)
	} else {
		log.Printf("[CaptureVersion] Nenhuma versão anterior, criando versão 1")
	}

	// Calcula hash do embedding se existir
	embeddingHash := ""
	if len(service.Embedding) > 0 {
		embeddingHash = vs.calculateEmbeddingHash(service.Embedding)
	}

	// Cria o snapshot da versão
	version := &models.ServiceVersion{
		ServiceID:             service.ID,
		VersionNumber:         versionNumber,
		CreatedAt:             time.Now().Unix(),
		CreatedBy:             createdBy,
		CreatedByCPF:          createdByCPF,
		ChangeType:            changeType,
		ChangeReason:          changeReason,
		PreviousVersion:       previousVersionNumber,
		IsRollback:            false,
		NomeServico:           service.NomeServico,
		OrgaoGestor:           service.OrgaoGestor,
		Resumo:                service.Resumo,
		TempoAtendimento:      service.TempoAtendimento,
		CustoServico:          service.CustoServico,
		ResultadoSolicitacao:  service.ResultadoSolicitacao,
		DescricaoCompleta:     service.DescricaoCompleta,
		Autor:                 service.Autor,
		DocumentosNecessarios: service.DocumentosNecessarios,
		InstrucoesSolicitante: service.InstrucoesSolicitante,
		CanaisDigitais:        service.CanaisDigitais,
		CanaisPresenciais:     service.CanaisPresenciais,
		ServicoNaoCobre:       service.ServicoNaoCobre,
		LegislacaoRelacionada: service.LegislacaoRelacionada,
		TemaGeral:             service.TemaGeral,
		PublicoEspecifico:     service.PublicoEspecifico,
		FixarDestaque:         service.FixarDestaque,
		AwaitingApproval:      service.AwaitingApproval,
		PublishedAt:           service.PublishedAt,
		IsFree:                service.IsFree,
		Status:                service.Status,
		SearchContent:         service.SearchContent,
		EmbeddingHash:         embeddingHash,
	}

	// Calcula diff se houver versão anterior
	if previousVersion != nil {
		changes := vs.ComputeDiff(previousVersion, version)
		if len(changes) > 0 {
			changesJSON, err := json.Marshal(changes)
			if err != nil {
				log.Printf("Erro ao serializar mudanças: %v", err)
			} else {
				version.ChangedFieldsJSON = string(changesJSON)
			}
		}
	} else {
		// Para a primeira versão, todas as mudanças são "create"
		changes := vs.GetAllFieldsAsChanges(version)
		if len(changes) > 0 {
			changesJSON, err := json.Marshal(changes)
			if err != nil {
				log.Printf("Erro ao serializar mudanças: %v", err)
			} else {
				version.ChangedFieldsJSON = string(changesJSON)
			}
		}
	}

	log.Printf("[CaptureVersion] Prestes a salvar versão: ServiceID=%s, VersionNumber=%d, CreatedBy='%s', CreatedByCPF='%s'",
		version.ServiceID, version.VersionNumber, version.CreatedBy, version.CreatedByCPF)

	// Salva a versão no Typesense
	savedVersion, err := vs.SaveVersion(ctx, version)
	if err != nil {
		log.Printf("[CaptureVersion] ERRO ao salvar versão: %v", err)
		return nil, err
	}

	log.Printf("[CaptureVersion] Versão salva com sucesso: ID=%s", savedVersion.ID)
	return savedVersion, nil
}

// ComputeDiff calcula as diferenças entre duas versões
func (vs *VersionService) ComputeDiff(oldVersion, newVersion *models.ServiceVersion) []models.FieldChange {
	changes := []models.FieldChange{}

	// Compara cada campo
	changes = append(changes, vs.compareField("nome_servico", oldVersion.NomeServico, newVersion.NomeServico)...)
	changes = append(changes, vs.compareField("orgao_gestor", oldVersion.OrgaoGestor, newVersion.OrgaoGestor)...)
	changes = append(changes, vs.compareField("resumo", oldVersion.Resumo, newVersion.Resumo)...)
	changes = append(changes, vs.compareField("tempo_atendimento", oldVersion.TempoAtendimento, newVersion.TempoAtendimento)...)
	changes = append(changes, vs.compareField("custo_servico", oldVersion.CustoServico, newVersion.CustoServico)...)
	changes = append(changes, vs.compareField("resultado_solicitacao", oldVersion.ResultadoSolicitacao, newVersion.ResultadoSolicitacao)...)
	changes = append(changes, vs.compareField("descricao_completa", oldVersion.DescricaoCompleta, newVersion.DescricaoCompleta)...)
	changes = append(changes, vs.compareField("autor", oldVersion.Autor, newVersion.Autor)...)
	changes = append(changes, vs.compareField("documentos_necessarios", oldVersion.DocumentosNecessarios, newVersion.DocumentosNecessarios)...)
	changes = append(changes, vs.compareField("instrucoes_solicitante", oldVersion.InstrucoesSolicitante, newVersion.InstrucoesSolicitante)...)
	changes = append(changes, vs.compareField("canais_digitais", oldVersion.CanaisDigitais, newVersion.CanaisDigitais)...)
	changes = append(changes, vs.compareField("canais_presenciais", oldVersion.CanaisPresenciais, newVersion.CanaisPresenciais)...)
	changes = append(changes, vs.compareField("servico_nao_cobre", oldVersion.ServicoNaoCobre, newVersion.ServicoNaoCobre)...)
	changes = append(changes, vs.compareField("legislacao_relacionada", oldVersion.LegislacaoRelacionada, newVersion.LegislacaoRelacionada)...)
	changes = append(changes, vs.compareField("tema_geral", oldVersion.TemaGeral, newVersion.TemaGeral)...)
	changes = append(changes, vs.compareField("publico_especifico", oldVersion.PublicoEspecifico, newVersion.PublicoEspecifico)...)
	changes = append(changes, vs.compareField("fixar_destaque", oldVersion.FixarDestaque, newVersion.FixarDestaque)...)
	changes = append(changes, vs.compareField("awaiting_approval", oldVersion.AwaitingApproval, newVersion.AwaitingApproval)...)
	changes = append(changes, vs.compareField("published_at", oldVersion.PublishedAt, newVersion.PublishedAt)...)
	changes = append(changes, vs.compareField("is_free", oldVersion.IsFree, newVersion.IsFree)...)
	changes = append(changes, vs.compareField("status", oldVersion.Status, newVersion.Status)...)
	changes = append(changes, vs.compareField("search_content", oldVersion.SearchContent, newVersion.SearchContent)...)

	return changes
}

// compareField compara um campo específico e retorna FieldChange se houver diferença
func (vs *VersionService) compareField(fieldName string, oldValue, newValue interface{}) []models.FieldChange {
	// Usa reflect para comparação profunda
	if !reflect.DeepEqual(oldValue, newValue) {
		valueType := vs.getValueType(newValue)
		return []models.FieldChange{{
			FieldName: fieldName,
			OldValue:  oldValue,
			NewValue:  newValue,
			ValueType: valueType,
		}}
	}
	return []models.FieldChange{}
}

// getValueType retorna o tipo de valor para FieldChange
func (vs *VersionService) getValueType(value interface{}) string {
	if value == nil {
		return "nil"
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Bool:
		return "bool"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Ptr:
		if v.IsNil() {
			return "nil"
		}
		return vs.getValueType(v.Elem().Interface())
	default:
		return "unknown"
	}
}

// GetAllFieldsAsChanges retorna todos os campos como mudanças (para versão 1)
func (vs *VersionService) GetAllFieldsAsChanges(version *models.ServiceVersion) []models.FieldChange {
	changes := []models.FieldChange{}

	// Adiciona todos os campos não-vazios como "novas" mudanças
	if version.NomeServico != "" {
		changes = append(changes, models.FieldChange{FieldName: "nome_servico", NewValue: version.NomeServico, ValueType: "string"})
	}
	if len(version.OrgaoGestor) > 0 {
		changes = append(changes, models.FieldChange{FieldName: "orgao_gestor", NewValue: version.OrgaoGestor, ValueType: "array"})
	}
	if version.Resumo != "" {
		changes = append(changes, models.FieldChange{FieldName: "resumo", NewValue: version.Resumo, ValueType: "string"})
	}
	if version.TemaGeral != "" {
		changes = append(changes, models.FieldChange{FieldName: "tema_geral", NewValue: version.TemaGeral, ValueType: "string"})
	}
	changes = append(changes, models.FieldChange{FieldName: "status", NewValue: version.Status, ValueType: "int"})

	return changes
}

// calculateEmbeddingHash calcula MD5 hash do embedding
func (vs *VersionService) calculateEmbeddingHash(embedding []float64) string {
	// Serializa o embedding para bytes
	data, err := json.Marshal(embedding)
	if err != nil {
		return ""
	}
	hash := md5.Sum(data)
	return fmt.Sprintf("%x", hash)
}

// SaveVersion salva uma versão no Typesense
func (vs *VersionService) SaveVersion(ctx context.Context, version *models.ServiceVersion) (*models.ServiceVersion, error) {
	log.Printf("[SaveVersion] Iniciando para ServiceID=%s, VersionNumber=%d", version.ServiceID, version.VersionNumber)

	// Garante que a collection existe antes de tentar salvar
	if err := vs.ensureCollectionExists(ctx); err != nil {
		log.Printf("[SaveVersion] Erro ao garantir que collection existe: %v", err)
		return nil, fmt.Errorf("erro ao criar/verificar collection service_versions: %v", err)
	}

	// Converte para map
	versionMap, err := vs.structToMap(version)
	if err != nil {
		log.Printf("[SaveVersion] Erro ao converter para map: %v", err)
		return nil, fmt.Errorf("erro ao converter versão para map: %v", err)
	}

	// Remove ID se vazio para auto-geração
	if version.ID == "" {
		delete(versionMap, "id")
		log.Printf("[SaveVersion] ID vazio removido para auto-geração")
	}

	log.Printf("[SaveVersion] Prestes a inserir no Typesense collection 'service_versions'")

	// Insere no Typesense
	result, err := vs.typesenseClient.Collection("service_versions").Documents().Create(ctx, versionMap, &api.DocumentIndexParameters{})
	if err != nil {
		log.Printf("[SaveVersion] ERRO do Typesense ao criar documento: %v", err)
		return nil, fmt.Errorf("erro ao salvar versão: %v", err)
	}

	log.Printf("[SaveVersion] Documento criado no Typesense com sucesso")

	// Converte resultado de volta para struct
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var savedVersion models.ServiceVersion
	if err := json.Unmarshal(resultBytes, &savedVersion); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	return &savedVersion, nil
}

// GetLatestVersion busca a última versão de um serviço
func (vs *VersionService) GetLatestVersion(ctx context.Context, serviceID string) (*models.ServiceVersion, error) {
	filterBy := fmt.Sprintf("service_id:=%s", serviceID)
	sortBy := "version_number:desc"

	searchParams := &api.SearchCollectionParams{
		Q:        pointer.String("*"),
		FilterBy: pointer.String(filterBy),
		SortBy:   pointer.String(sortBy),
		PerPage:  pointer.Int(1),
	}

	result, err := vs.typesenseClient.Collection("service_versions").Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar última versão: %v", err)
	}

	// Parse resultado
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var searchResult struct {
		Hits []struct {
			Document models.ServiceVersion `json:"document"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(resultBytes, &searchResult); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	if len(searchResult.Hits) == 0 {
		return nil, nil // Nenhuma versão encontrada
	}

	return &searchResult.Hits[0].Document, nil
}

// GetVersionByNumber busca uma versão específica de um serviço
func (vs *VersionService) GetVersionByNumber(ctx context.Context, serviceID string, versionNumber int64) (*models.ServiceVersion, error) {
	filterBy := fmt.Sprintf("service_id:=%s && version_number:=%d", serviceID, versionNumber)
	log.Printf("[GetVersionByNumber] Buscando serviceID='%s', versionNumber=%d, filterBy='%s'", serviceID, versionNumber, filterBy)

	searchParams := &api.SearchCollectionParams{
		Q:        pointer.String("*"),
		FilterBy: pointer.String(filterBy),
		PerPage:  pointer.Int(1),
	}

	result, err := vs.typesenseClient.Collection("service_versions").Documents().Search(ctx, searchParams)
	if err != nil {
		log.Printf("[GetVersionByNumber] Erro ao buscar: %v", err)
		return nil, fmt.Errorf("erro ao buscar versão: %v", err)
	}

	// Parse resultado
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var searchResult struct {
		Found int `json:"found"`
		Hits  []struct {
			Document models.ServiceVersion `json:"document"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(resultBytes, &searchResult); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	log.Printf("[GetVersionByNumber] Encontrado %d resultado(s)", searchResult.Found)

	if len(searchResult.Hits) == 0 {
		return nil, fmt.Errorf("versão %d não encontrada", versionNumber)
	}

	log.Printf("[GetVersionByNumber] Retornando versão ID=%s", searchResult.Hits[0].Document.ID)
	return &searchResult.Hits[0].Document, nil
}

// ListVersions lista todas as versões de um serviço com paginação
func (vs *VersionService) ListVersions(ctx context.Context, serviceID string, page, perPage int) (*models.VersionHistory, error) {
	filterBy := fmt.Sprintf("service_id:=%s", serviceID)
	sortBy := "version_number:desc"

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}

	searchParams := &api.SearchCollectionParams{
		Q:        pointer.String("*"),
		FilterBy: pointer.String(filterBy),
		SortBy:   pointer.String(sortBy),
		Page:     pointer.Int(page),
		PerPage:  pointer.Int(perPage),
	}

	result, err := vs.typesenseClient.Collection("service_versions").Documents().Search(ctx, searchParams)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar versões: %v", err)
	}

	// Parse resultado
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar resultado: %v", err)
	}

	var searchResult struct {
		Found int `json:"found"`
		OutOf int `json:"out_of"`
		Hits  []struct {
			Document models.ServiceVersion `json:"document"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(resultBytes, &searchResult); err != nil {
		return nil, fmt.Errorf("erro ao deserializar resultado: %v", err)
	}

	versions := make([]models.ServiceVersion, len(searchResult.Hits))
	for i, hit := range searchResult.Hits {
		versions[i] = hit.Document
	}

	return &models.VersionHistory{
		Found:    searchResult.Found,
		OutOf:    searchResult.OutOf,
		Page:     page,
		Versions: versions,
	}, nil
}

// CompareVersions compara duas versões e retorna o diff
func (vs *VersionService) CompareVersions(ctx context.Context, serviceID string, fromVersion, toVersion int64) (*models.VersionDiff, error) {
	// Busca as duas versões
	oldVer, err := vs.GetVersionByNumber(ctx, serviceID, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar versão %d: %v", fromVersion, err)
	}

	newVer, err := vs.GetVersionByNumber(ctx, serviceID, toVersion)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar versão %d: %v", toVersion, err)
	}

	// Computa diff
	changes := vs.ComputeDiff(oldVer, newVer)

	return &models.VersionDiff{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Changes:     changes,
		ChangedBy:   newVer.CreatedBy,
		ChangedAt:   newVer.CreatedAt,
		ChangeType:  newVer.ChangeType,
	}, nil
}

// ensureCollectionExists garante que a collection service_versions existe
func (vs *VersionService) ensureCollectionExists(ctx context.Context) error {
	// Verifica se a collection já existe
	_, err := vs.typesenseClient.Collection("service_versions").Retrieve(ctx)
	if err == nil {
		// Collection já existe
		return nil
	}

	// Se não existe (404), cria a collection
	errMsg := err.Error()
	if !strings.Contains(errMsg, "404") && !strings.Contains(errMsg, "Not found") && !strings.Contains(errMsg, "Not Found") {
		// Erro que não é 404 - retorna o erro
		return err
	}

	log.Printf("[ensureCollectionExists] Collection service_versions não existe, criando...")

	// Cria a collection
	schema := &api.CollectionSchema{
		Name: "service_versions",
		Fields: []api.Field{
			{Name: "service_id", Type: "string", Facet: pointer.True()},
			{Name: "version_number", Type: "int64"},
			{Name: "created_at", Type: "int64", Sort: pointer.True()},
			{Name: "created_by", Type: "string"},
			{Name: "created_by_cpf", Type: "string", Facet: pointer.True()},
			{Name: "change_type", Type: "string", Facet: pointer.True()},
			{Name: "change_reason", Type: "string", Optional: pointer.True()},
			{Name: "previous_version", Type: "int64", Optional: pointer.True()},
			{Name: "is_rollback", Type: "bool", Facet: pointer.True()},
			{Name: "rollback_to_version", Type: "int64", Optional: pointer.True()},
			{Name: "nome_servico", Type: "string"},
			{Name: "orgao_gestor", Type: "string[]", Facet: pointer.True()},
			{Name: "resumo", Type: "string"},
			{Name: "tempo_atendimento", Type: "string", Optional: pointer.True()},
			{Name: "custo_servico", Type: "string", Optional: pointer.True()},
			{Name: "resultado_solicitacao", Type: "string", Optional: pointer.True()},
			{Name: "descricao_completa", Type: "string", Optional: pointer.True()},
			{Name: "autor", Type: "string"},
			{Name: "documentos_necessarios", Type: "string[]", Optional: pointer.True()},
			{Name: "instrucoes_solicitante", Type: "string", Optional: pointer.True()},
			{Name: "canais_digitais", Type: "string[]", Optional: pointer.True()},
			{Name: "canais_presenciais", Type: "string[]", Optional: pointer.True()},
			{Name: "servico_nao_cobre", Type: "string", Optional: pointer.True()},
			{Name: "legislacao_relacionada", Type: "string[]", Optional: pointer.True()},
			{Name: "tema_geral", Type: "string", Facet: pointer.True()},
			{Name: "publico_especifico", Type: "string[]", Optional: pointer.True(), Facet: pointer.True()},
			{Name: "fixar_destaque", Type: "bool", Facet: pointer.True()},
			{Name: "awaiting_approval", Type: "bool", Facet: pointer.True()},
			{Name: "published_at", Type: "int64", Optional: pointer.True()},
			{Name: "is_free", Type: "bool", Optional: pointer.True(), Facet: pointer.True()},
			{Name: "status", Type: "int32", Facet: pointer.True()},
			{Name: "search_content", Type: "string"},
			{Name: "embedding_hash", Type: "string", Optional: pointer.True()},
			{Name: "changed_fields_json", Type: "string", Optional: pointer.True()},
		},
		DefaultSortingField: pointer.String("created_at"),
	}

	_, err = vs.typesenseClient.Collections().Create(ctx, schema)
	if err != nil {
		log.Printf("[ensureCollectionExists] Erro ao criar collection: %v", err)
		return fmt.Errorf("erro ao criar collection service_versions: %v", err)
	}

	log.Printf("[ensureCollectionExists] Collection service_versions criada com sucesso")
	return nil
}

// structToMap converte struct para map[string]interface{}
func (vs *VersionService) structToMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}
