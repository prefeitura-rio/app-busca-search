package models

// FieldChange representa uma mudança em um campo específico
type FieldChange struct {
	FieldName string      `json:"field_name" typesense:"field_name"`
	OldValue  interface{} `json:"old_value,omitempty" typesense:"old_value,optional"`
	NewValue  interface{} `json:"new_value,omitempty" typesense:"new_value,optional"`
	ValueType string      `json:"value_type" typesense:"value_type"` // "string", "int", "bool", "array", "object"
}

// ServiceVersion representa uma versão completa de um serviço
type ServiceVersion struct {
	ID              string         `json:"id,omitempty" typesense:"id,optional"`
	ServiceID       string         `json:"service_id" validate:"required" typesense:"service_id"`
	VersionNumber   int64          `json:"version_number" validate:"required" typesense:"version_number"`
	CreatedAt       int64          `json:"created_at" typesense:"created_at"`
	CreatedBy       string         `json:"created_by" validate:"required" typesense:"created_by"`
	CreatedByCPF    string         `json:"created_by_cpf" validate:"required" typesense:"created_by_cpf"`
	ChangeType      string         `json:"change_type" validate:"required,oneof=create update publish unpublish delete rollback" typesense:"change_type"`
	ChangeReason    string         `json:"change_reason,omitempty" typesense:"change_reason,optional"`
	PreviousVersion int64          `json:"previous_version,omitempty" typesense:"previous_version,optional"`
	IsRollback      bool           `json:"is_rollback" typesense:"is_rollback"`
	RollbackToVersion int64        `json:"rollback_to_version,omitempty" typesense:"rollback_to_version,optional"`

	// Snapshot completo do serviço (sem embedding para economizar espaço)
	NomeServico               string                 `json:"nome_servico" typesense:"nome_servico"`
	OrgaoGestor               []string               `json:"orgao_gestor" typesense:"orgao_gestor"`
	Resumo                    string                 `json:"resumo" typesense:"resumo"`
	TempoAtendimento          string                 `json:"tempo_atendimento,omitempty" typesense:"tempo_atendimento,optional"`
	CustoServico              string                 `json:"custo_servico,omitempty" typesense:"custo_servico,optional"`
	ResultadoSolicitacao      string                 `json:"resultado_solicitacao,omitempty" typesense:"resultado_solicitacao,optional"`
	DescricaoCompleta         string                 `json:"descricao_completa,omitempty" typesense:"descricao_completa,optional"`
	Autor                     string                 `json:"autor" typesense:"autor"`
	DocumentosNecessarios     []string               `json:"documentos_necessarios,omitempty" typesense:"documentos_necessarios,optional"`
	InstrucoesSolicitante     string                 `json:"instrucoes_solicitante,omitempty" typesense:"instrucoes_solicitante,optional"`
	CanaisDigitais            []string               `json:"canais_digitais,omitempty" typesense:"canais_digitais,optional"`
	CanaisPresenciais         []string               `json:"canais_presenciais,omitempty" typesense:"canais_presenciais,optional"`
	ServicoNaoCobre           string                 `json:"servico_nao_cobre,omitempty" typesense:"servico_nao_cobre,optional"`
	LegislacaoRelacionada     []string               `json:"legislacao_relacionada,omitempty" typesense:"legislacao_relacionada,optional"`
	TemaGeral                 string                 `json:"tema_geral" typesense:"tema_geral"`
	PublicoEspecifico         []string               `json:"publico_especifico,omitempty" typesense:"publico_especifico,optional"`
	FixarDestaque             bool                   `json:"fixar_destaque" typesense:"fixar_destaque"`
	AwaitingApproval          bool                   `json:"awaiting_approval" typesense:"awaiting_approval"`
	PublishedAt               *int64                 `json:"published_at,omitempty" typesense:"published_at,optional"`
	IsFree                    *bool                  `json:"is_free,omitempty" typesense:"is_free,optional"`
	Status                    int                    `json:"status" typesense:"status"`
	SearchContent             string                 `json:"search_content" typesense:"search_content"`

	// Hash do embedding para verificação (não armazenamos o embedding completo)
	EmbeddingHash   string `json:"embedding_hash,omitempty" typesense:"embedding_hash,optional"`

	// Campos de mudança (armazenados como JSON string no Typesense)
	ChangedFieldsJSON string `json:"changed_fields_json,omitempty" typesense:"changed_fields_json,optional"`
}

// VersionDiff representa a diferença entre duas versões
type VersionDiff struct {
	FromVersion int64         `json:"from_version"`
	ToVersion   int64         `json:"to_version"`
	Changes     []FieldChange `json:"changes"`
	ChangedBy   string        `json:"changed_by"`
	ChangedAt   int64         `json:"changed_at"`
	ChangeType  string        `json:"change_type"`
}

// RollbackRequest representa uma solicitação de rollback
type RollbackRequest struct {
	ToVersion    int64  `json:"to_version" validate:"required,min=1"`
	ChangeReason string `json:"change_reason,omitempty"`
}

// VersionHistory representa uma lista paginada de versões
type VersionHistory struct {
	Found    int              `json:"found"`
	OutOf    int              `json:"out_of"`
	Page     int              `json:"page"`
	Versions []ServiceVersion `json:"versions"`
}

// VersionCompareRequest representa uma solicitação de comparação entre versões
type VersionCompareRequest struct {
	FromVersion int64 `json:"from_version" validate:"required,min=1"`
	ToVersion   int64 `json:"to_version" validate:"required,min=1"`
}

// AuditLogFilter representa filtros para consulta de audit log
type AuditLogFilter struct {
	ServiceID    string `json:"service_id,omitempty"`
	UserCPF      string `json:"user_cpf,omitempty"`
	UserName     string `json:"user_name,omitempty"`
	ChangeType   string `json:"change_type,omitempty"`
	StartDate    int64  `json:"start_date,omitempty"`
	EndDate      int64  `json:"end_date,omitempty"`
	Page         int    `json:"page"`
	PerPage      int    `json:"per_page"`
}
