package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/prefeitura-rio/app-busca-search/internal/config"
	"github.com/prefeitura-rio/app-busca-search/internal/migration/schemas"
	"github.com/prefeitura-rio/app-busca-search/internal/models"
	"github.com/prefeitura-rio/app-busca-search/internal/services"
	"github.com/typesense/typesense-go/v3/typesense"
)

var (
	schemaVersion  = flag.String("schema", "", "Versão do schema para migração (ex: v2)")
	migrationID    = flag.String("id", "", "ID da migração para rollback específico")
	dryRun         = flag.Bool("dry-run", false, "Executa simulação sem modificar dados")
	page           = flag.Int("page", 1, "Página para listagem de histórico")
	perPage        = flag.Int("per-page", 10, "Itens por página para listagem de histórico")
	userName       = flag.String("user", "CLI", "Nome do usuário que está executando")
	jsonOutput     = flag.Bool("json", false, "Saída em formato JSON")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Uso: %s <comando> [opções]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Comandos disponíveis:\n")
		fmt.Fprintf(os.Stderr, "  start     Inicia uma migração de schema\n")
		fmt.Fprintf(os.Stderr, "  status    Verifica o status da migração atual\n")
		fmt.Fprintf(os.Stderr, "  rollback  Reverte para a versão anterior\n")
		fmt.Fprintf(os.Stderr, "  history   Lista o histórico de migrações\n")
		fmt.Fprintf(os.Stderr, "  schemas   Lista os schemas disponíveis\n")
		fmt.Fprintf(os.Stderr, "\nOpções:\n")
		flag.PrintDefaults()
	}

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	command := os.Args[1]
	os.Args = append(os.Args[:1], os.Args[2:]...)
	flag.Parse()

	cfg := config.LoadConfig()

	// Cliente Typesense com timeout maior para operações de migração (10 minutos)
	typesenseClient := typesense.NewClient(
		typesense.WithServer(fmt.Sprintf("%s://%s:%s", cfg.TypesenseProtocol, cfg.TypesenseHost, cfg.TypesensePort)),
		typesense.WithAPIKey(cfg.TypesenseAPIKey),
		typesense.WithConnectionTimeout(10*time.Minute),
	)

	schemaRegistry := schemas.NewRegistry()
	migrationService := services.NewMigrationService(typesenseClient, schemaRegistry)

	ctx := context.Background()

	switch command {
	case "start":
		cmdStart(ctx, migrationService)
	case "status":
		cmdStatus(ctx, migrationService)
	case "rollback":
		cmdRollback(ctx, migrationService)
	case "history":
		cmdHistory(ctx, migrationService)
	case "schemas":
		cmdSchemas(ctx, schemaRegistry, migrationService)
	default:
		fmt.Fprintf(os.Stderr, "Comando desconhecido: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func cmdStart(ctx context.Context, ms *services.MigrationService) {
	if *schemaVersion == "" {
		fmt.Fprintln(os.Stderr, "Erro: --schema é obrigatório para o comando start")
		fmt.Fprintln(os.Stderr, "Exemplo: migrate start --schema=v2")
		os.Exit(1)
	}

	req := &models.MigrationStartRequest{
		SchemaVersion: *schemaVersion,
		DryRun:        *dryRun,
	}

	fmt.Printf("🚀 Iniciando migração para schema %s\n", *schemaVersion)
	if *dryRun {
		fmt.Println("⚠️  Modo dry-run ativado - nenhuma alteração será feita")
	}

	response, err := ms.StartMigration(ctx, req, *userName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Erro ao iniciar migração: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		printJSON(response)
		return
	}

	if response.Status == models.MigrationStatusCompleted {
		fmt.Println("\n✅ Migração concluída com sucesso!")
	} else if response.Status == models.MigrationStatusFailed {
		fmt.Println("\n❌ Migração falhou!")
	} else {
		fmt.Println("\n✅ Migração iniciada!")
	}

	fmt.Printf("   Status: %s\n", formatStatus(response.Status))
	fmt.Printf("   Schema: %s\n", response.SchemaVersion)
	if response.TargetCollection != "" {
		fmt.Printf("   Collection destino: %s\n", response.TargetCollection)
	}
	if response.BackupCollection != "" {
		fmt.Printf("   Backup: %s\n", response.BackupCollection)
	}
	fmt.Printf("   Documentos: %d/%d\n", response.MigratedDocuments, response.TotalDocuments)

	if response.ErrorMessage != "" {
		fmt.Printf("   Erro: %s\n", response.ErrorMessage)
	}
}

func cmdStatus(ctx context.Context, ms *services.MigrationService) {
	response, err := ms.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Erro ao obter status: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		printJSON(response)
		return
	}

	fmt.Println("📊 Status da Migração")
	fmt.Println("---------------------")
	fmt.Printf("Status: %s\n", formatStatus(response.Status))
	fmt.Printf("Bloqueado: %v\n", response.IsLocked)

	if response.Status != models.MigrationStatusIdle {
		fmt.Printf("Schema: %s\n", response.SchemaVersion)
		fmt.Printf("Collection origem: %s\n", response.SourceCollection)
		fmt.Printf("Collection destino: %s\n", response.TargetCollection)
		fmt.Printf("Backup: %s\n", response.BackupCollection)
		fmt.Printf("Iniciado em: %s\n", formatTimestamp(response.StartedAt))
		fmt.Printf("Iniciado por: %s\n", response.StartedBy)
		fmt.Printf("Progresso: %.1f%% (%d/%d)\n", response.Progress, response.MigratedDocuments, response.TotalDocuments)

		if response.CompletedAt > 0 {
			fmt.Printf("Completado em: %s\n", formatTimestamp(response.CompletedAt))
		}

		if response.ErrorMessage != "" {
			fmt.Printf("Erro: %s\n", response.ErrorMessage)
		}
	}
}

func cmdRollback(ctx context.Context, ms *services.MigrationService) {
	req := &models.MigrationRollbackRequest{
		MigrationID: *migrationID,
	}

	fmt.Println("🔄 Iniciando rollback...")

	response, err := ms.RollbackMigration(ctx, req, *userName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Erro ao executar rollback: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		printJSON(response)
		return
	}

	fmt.Println("\n✅ Rollback concluído com sucesso!")
	fmt.Printf("   Schema restaurado: %s\n", response.SchemaVersion)
	fmt.Printf("   Collection ativa: %s\n", response.TargetCollection)
	fmt.Printf("   Documentos: %d\n", response.TotalDocuments)
}

func cmdHistory(ctx context.Context, ms *services.MigrationService) {
	response, err := ms.GetHistory(ctx, *page, *perPage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Erro ao obter histórico: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		printJSON(response)
		return
	}

	fmt.Printf("📜 Histórico de Migrações (página %d, %d resultados)\n", *page, response.Found)
	fmt.Println("---------------------------------------------------")

	if len(response.Migrations) == 0 {
		fmt.Println("Nenhuma migração encontrada.")
		return
	}

	for _, m := range response.Migrations {
		fmt.Printf("\n[%s] %s\n", m.ID, formatStatus(m.Status))
		fmt.Printf("   Schema: %s", m.SchemaVersion)
		if m.PreviousSchemaVersion != "" {
			fmt.Printf(" (anterior: %s)", m.PreviousSchemaVersion)
		}
		fmt.Println()
		fmt.Printf("   Iniciado: %s por %s\n", formatTimestamp(m.StartedAt), m.StartedBy)
		if m.CompletedAt > 0 {
			fmt.Printf("   Completado: %s\n", formatTimestamp(m.CompletedAt))
		}
		fmt.Printf("   Documentos: %d\n", m.TotalDocuments)
		if m.ErrorMessage != "" {
			fmt.Printf("   Erro: %s\n", m.ErrorMessage)
		}
	}
}

func cmdSchemas(ctx context.Context, registry *schemas.Registry, ms *services.MigrationService) {
	versions := registry.ListVersions()
	
	// Consulta a versão real em uso no Typesense
	currentVersion := ms.GetCurrentSchemaVersion(ctx)

	if *jsonOutput {
		printJSON(map[string]interface{}{
			"current_version":    currentVersion,
			"available_versions": versions,
		})
		return
	}

	fmt.Println("📋 Schemas Disponíveis")
	fmt.Println("---------------------")
	fmt.Printf("Versão em uso: %s (consultado do Typesense)\n\n", currentVersion)
	fmt.Println("Versões disponíveis:")
	for _, v := range versions {
		marker := "  "
		if v == currentVersion {
			marker = "* "
		}
		fmt.Printf("%s%s\n", marker, v)
	}
}

func formatStatus(status models.MigrationStatus) string {
	switch status {
	case models.MigrationStatusIdle:
		return "🔵 Ocioso"
	case models.MigrationStatusInProgress:
		return "🟡 Em progresso"
	case models.MigrationStatusCompleted:
		return "🟢 Concluído"
	case models.MigrationStatusFailed:
		return "🔴 Falhou"
	case models.MigrationStatusRollback:
		return "🟠 Rollback"
	default:
		return string(status)
	}
}

func formatTimestamp(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("02/01/2006 15:04:05")
}

func printJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatalf("Erro ao serializar JSON: %v", err)
	}
	fmt.Println(string(data))
}

