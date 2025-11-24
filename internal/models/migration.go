package models

// MigrationStatus representa os possíveis estados de uma migração
type MigrationStatus string

const (
	MigrationStatusIdle       MigrationStatus = "idle"
	MigrationStatusInProgress MigrationStatus = "in_progress"
	MigrationStatusCompleted  MigrationStatus = "completed"
	MigrationStatusFailed     MigrationStatus = "failed"
	MigrationStatusRollback   MigrationStatus = "rollback"
)

// MigrationControl representa o estado de controle de uma migração
type MigrationControl struct {
	ID                   string          `json:"id,omitempty" typesense:"id,optional"`
	Status               MigrationStatus `json:"status" typesense:"status"`
	SourceCollection     string          `json:"source_collection" typesense:"source_collection"`
	TargetCollection     string          `json:"target_collection" typesense:"target_collection"`
	BackupCollection     string          `json:"backup_collection" typesense:"backup_collection"`
	SchemaVersion        string          `json:"schema_version" typesense:"schema_version"`
	PreviousSchemaVersion string         `json:"previous_schema_version,omitempty" typesense:"previous_schema_version,optional"`
	StartedAt            int64           `json:"started_at" typesense:"started_at"`
	CompletedAt          int64           `json:"completed_at,omitempty" typesense:"completed_at,optional"`
	StartedBy            string          `json:"started_by" typesense:"started_by"`
	StartedByCPF         string          `json:"started_by_cpf,omitempty" typesense:"started_by_cpf,optional"`
	TotalDocuments       int             `json:"total_documents" typesense:"total_documents"`
	MigratedDocuments    int             `json:"migrated_documents" typesense:"migrated_documents"`
	ErrorMessage         string          `json:"error_message,omitempty" typesense:"error_message,optional"`
	IsLocked             bool            `json:"is_locked" typesense:"is_locked"`
}

// MigrationStartRequest representa uma solicitação de início de migração
type MigrationStartRequest struct {
	SchemaVersion string `json:"schema_version" validate:"required"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

// MigrationStatusResponse representa a resposta de status de migração
type MigrationStatusResponse struct {
	Status            MigrationStatus `json:"status"`
	SchemaVersion     string          `json:"schema_version,omitempty"`
	SourceCollection  string          `json:"source_collection,omitempty"`
	TargetCollection  string          `json:"target_collection,omitempty"`
	BackupCollection  string          `json:"backup_collection,omitempty"`
	StartedAt         int64           `json:"started_at,omitempty"`
	CompletedAt       int64           `json:"completed_at,omitempty"`
	StartedBy         string          `json:"started_by,omitempty"`
	TotalDocuments    int             `json:"total_documents,omitempty"`
	MigratedDocuments int             `json:"migrated_documents,omitempty"`
	Progress          float64         `json:"progress,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	IsLocked          bool            `json:"is_locked"`
}

// MigrationHistoryItem representa um item no histórico de migrações
type MigrationHistoryItem struct {
	ID                    string          `json:"id"`
	Status                MigrationStatus `json:"status"`
	SchemaVersion         string          `json:"schema_version"`
	PreviousSchemaVersion string          `json:"previous_schema_version,omitempty"`
	StartedAt             int64           `json:"started_at"`
	CompletedAt           int64           `json:"completed_at,omitempty"`
	StartedBy             string          `json:"started_by"`
	TotalDocuments        int             `json:"total_documents"`
	ErrorMessage          string          `json:"error_message,omitempty"`
}

// MigrationHistoryResponse representa a resposta de histórico de migrações
type MigrationHistoryResponse struct {
	Found      int                    `json:"found"`
	OutOf      int                    `json:"out_of"`
	Page       int                    `json:"page"`
	Migrations []MigrationHistoryItem `json:"migrations"`
}

// MigrationRollbackRequest representa uma solicitação de rollback
type MigrationRollbackRequest struct {
	MigrationID string `json:"migration_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

