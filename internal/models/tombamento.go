package models

// Tombamento representa um mapeamento de serviço antigo para serviço novo
type Tombamento struct {
	ID                string `json:"id,omitempty" typesense:"id,optional"`
	Origem            string `json:"origem" validate:"required,oneof=1746_v2_llm carioca-digital_v2_llm" typesense:"origem"`
	IDServicoAntigo   string `json:"id_servico_antigo" validate:"required,max=20000" typesense:"id_servico_antigo"`
	IDServicoNovo     string `json:"id_servico_novo" validate:"required,max=20000" typesense:"id_servico_novo"`
	CriadoEm          int64  `json:"criado_em" typesense:"criado_em"`
	CriadoPor         string `json:"criado_por" validate:"required,max=20000" typesense:"criado_por"`
	Observacoes       string `json:"observacoes,omitempty" validate:"max=20000" typesense:"observacoes,optional"`
}

// TombamentoRequest representa os dados de entrada para criar/atualizar um tombamento
type TombamentoRequest struct {
	Origem          string `json:"origem" validate:"required,oneof=1746_v2_llm carioca-digital_v2_llm"`
	IDServicoAntigo string `json:"id_servico_antigo" validate:"required,max=20000"`
	IDServicoNovo   string `json:"id_servico_novo" validate:"required,max=20000"`
	Observacoes     string `json:"observacoes,omitempty" validate:"max=20000"`
}

// TombamentoResponse representa a resposta de listagem de tombamentos
type TombamentoResponse struct {
	Found       int          `json:"found"`
	OutOf       int          `json:"out_of"`
	Page        int          `json:"page"`
	Tombamentos []Tombamento `json:"tombamentos"`
}
