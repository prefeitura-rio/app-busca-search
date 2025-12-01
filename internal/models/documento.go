package models

import (
	"encoding/json"

	"github.com/prefeitura-rio/app-busca-search/internal/utils"
)

// EtapaDetalhada representa uma etapa detalhada do serviço
type EtapaDetalhada struct {
	Descricao       string `json:"descricao"`
	Ordem           int64  `json:"ordem"`
	TemLink         bool   `json:"tem_link"`
	LinkSolicitacao string `json:"link_solicitacao,omitempty"`
	EmManutencao    bool   `json:"em_manutencao"`
}

// DocumentoDetalhado representa um documento detalhado necessário
type DocumentoDetalhado struct {
	Descricao     string               `json:"descricao"`
	Ordem         int64                `json:"ordem"`
	TemURL        bool                 `json:"tem_url"`
	URL           string               `json:"url,omitempty"`
	Obrigatorio   bool                 `json:"obrigatorio"`
	PermiteUpload bool                 `json:"permite_upload"`
	UploadInfo    *DocumentoUploadInfo `json:"upload_info,omitempty"`
}

// DocumentoUploadInfo representa informações de upload para documentos
type DocumentoUploadInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	Alt         string `json:"alt"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Date        string `json:"date"`
	Modified    string `json:"modified"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Subtype     string `json:"subtype"`
	MimeType    string `json:"mime_type"`
	Link        string `json:"link"`
	URL         string `json:"url"`
	Icon        string `json:"icon"`
	Filesize    int64  `json:"filesize"`
	MenuOrder   int64  `json:"menu_order"`
	UploadedTo  int64  `json:"uploaded_to"`
}

// Documento representa um documento completo baseado no novo schema do Typesense
type Documento struct {
	ID                        string               `json:"id,omitempty"`
	Titulo                    string               `json:"titulo"`
	Descricao                 string               `json:"descricao"`
	URL                       string               `json:"url"`
	Collection                string               `json:"collection"`
	Category                  string               `json:"category"`
	UltimaAtualizacao         string               `json:"ultima_atualizacao"`
	Slug                      string               `json:"slug"`
	Status                    string               `json:"status"`
	Tipo                      string               `json:"tipo"`
	OrgaoGestor               string               `json:"orgao_gestor"`
	NomeGestor                *string              `json:"nome_gestor,omitempty"`
	InformacoesComplementares *string              `json:"informacoes_complementares,omitempty"`
	EsteServicoNaoCobre       *string              `json:"este_servico_nao_cobre,omitempty"`
	ProcedimentosEspeciais    *string              `json:"procedimentos_especiais,omitempty"`
	LinkAcesso                *string              `json:"link_acesso,omitempty"`
	DisponiveApp              bool                 `json:"disponivel_app"`
	AppAndroid                *string              `json:"app_android,omitempty"`
	AppIOS                    *string              `json:"app_ios,omitempty"`
	AtendimentoPresencial     bool                 `json:"atendimento_presencial"`
	LocalPresencial           *string              `json:"local_presencial,omitempty"`
	EmManutencao              bool                 `json:"em_manutencao"`
	Etapas                    []string             `json:"etapas,omitempty"`
	EtapasDetalhadas          []EtapaDetalhada     `json:"etapas_detalhadas,omitempty"`
	DocumentosNecessarios     []string             `json:"documentos_necessarios,omitempty"`
	DocumentosDetalhados      []DocumentoDetalhado `json:"documentos_detalhados,omitempty"`
	PermiteUpload             bool                 `json:"permite_upload"`
	PrazoEsperado             *string              `json:"prazo_esperado,omitempty"`
	CustoDoServico            string               `json:"custo_do_servico"`
	ValorASerPago             *string              `json:"valor_a_ser_pago,omitempty"`
	Gratuito                  bool                 `json:"gratuito"`
	Temas                     []string             `json:"temas"`
	PublicoAlvo               []string             `json:"publico_alvo"`
	Atividades                []string             `json:"atividades"`
	Sistema                   string               `json:"sistema"`
	TemLegislacao             bool                 `json:"tem_legislacao"`
	Legislacao                []string             `json:"legislacao,omitempty"`
	SearchContent             string               `json:"search_content"`
	PalavrasChave             []string             `json:"palavras_chave,omitempty"`
	Destaque                  bool                 `json:"destaque"`
	ScoreCompletude           *float64             `json:"score_completude,omitempty"`

	// Campos de embedding (geralmente não retornados nas consultas)
	Embedding []float64 `json:"embedding,omitempty"`
}

// DocumentoResumo representa um documento com apenas título e ID para busca por categoria
type DocumentoResumo struct {
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
}

// BuscaCategoriaResponse representa a resposta da busca por categoria
type BuscaCategoriaResponse struct {
	Found      int               `json:"found"`
	OutOf      int               `json:"out_of"`
	Page       int               `json:"page"`
	Documentos []DocumentoResumo `json:"documentos"`
}

// AgentsConfig representa a configuração para agentes
type AgentsConfig struct {
	ToolHint           string `json:"tool_hint" typesense:"tool_hint,optional"`
	ExclusiveForAgents bool   `json:"exclusive_for_agents" typesense:"exclusive_for_agents"`
}

// Button representa um botão de ação para o serviço
type Button struct {
	Titulo     string `json:"titulo"`
	Descricao  string `json:"descricao"`
	IsEnabled  bool   `json:"is_enabled"`
	Ordem      int    `json:"ordem"`
	URLService string `json:"url_service"`
}

// PrefRioService representa um serviço da collection prefrio_services_base
type PrefRioService struct {
	ID                    string                 `json:"id,omitempty" typesense:"id,optional"`
	NomeServico           string                 `json:"nome_servico" validate:"required,max=20000" typesense:"nome_servico"`
	OrgaoGestor           []string               `json:"orgao_gestor" validate:"required,min=1" typesense:"orgao_gestor"`
	Resumo                string                 `json:"resumo" validate:"required,max=20000" typesense:"resumo"`
	TempoAtendimento      string                 `json:"tempo_atendimento" validate:"required,max=20000" typesense:"tempo_atendimento"`
	CustoServico          string                 `json:"custo_servico" validate:"required,max=20000" typesense:"custo_servico"`
	ResultadoSolicitacao  string                 `json:"resultado_solicitacao" validate:"required,max=20000" typesense:"resultado_solicitacao"`
	DescricaoCompleta     string                 `json:"descricao_completa" validate:"required,max=20000" typesense:"descricao_completa"`
	Autor                 string                 `json:"autor" validate:"required,max=20000" typesense:"autor"`
	DocumentosNecessarios []string               `json:"documentos_necessarios" typesense:"documentos_necessarios,optional"`
	InstrucoesSolicitante string                 `json:"instrucoes_solicitante" validate:"max=20000" typesense:"instrucoes_solicitante,optional"`
	CanaisDigitais        []string               `json:"canais_digitais" typesense:"canais_digitais,optional"`
	CanaisPresenciais     []string               `json:"canais_presenciais" typesense:"canais_presenciais,optional"`
	ServicoNaoCobre       string                 `json:"servico_nao_cobre" validate:"max=20000" typesense:"servico_nao_cobre,optional"`
	LegislacaoRelacionada []string               `json:"legislacao_relacionada" typesense:"legislacao_relacionada,optional"`
	TemaGeral             string                 `json:"tema_geral" validate:"required,max=20000" typesense:"tema_geral"`
	SubCategoria          *string                `json:"sub_categoria,omitempty" typesense:"sub_categoria,optional"`
	PublicoEspecifico     []string               `json:"publico_especifico,omitempty" typesense:"publico_especifico,optional"`
	FixarDestaque         bool                   `json:"fixar_destaque" typesense:"fixar_destaque"`
	AwaitingApproval      bool                   `json:"awaiting_approval" typesense:"awaiting_approval"`
	PublishedAt           *int64                 `json:"published_at,omitempty" typesense:"published_at,optional"`
	IsFree                *bool                  `json:"is_free,omitempty" typesense:"is_free,optional"`
	Agents                *AgentsConfig          `json:"agents,omitempty" typesense:"agents,optional"`
	ExtraFields           map[string]interface{} `json:"extra_fields,omitempty" typesense:"extra_fields,optional"`
	Status                int                    `json:"status" validate:"min=0,max=1" typesense:"status"` // 0=Draft, 1=Published
	CreatedAt             int64                  `json:"created_at" typesense:"created_at"`
	LastUpdate            int64                  `json:"last_update" typesense:"last_update"`
	SearchContent         string                 `json:"search_content" typesense:"search_content"`
	Buttons               []Button               `json:"buttons" typesense:"buttons,optional"`
	Embedding             []float64              `json:"embedding,omitempty" typesense:"embedding,optional"`
}

// MarshalJSON customiza a serialização JSON para adicionar campos plaintext
func (s *PrefRioService) MarshalJSON() ([]byte, error) {
	// Cria um alias para evitar recursão infinita
	type Alias PrefRioService

	// Cria estrutura auxiliar com todos os campos originais mais os plaintext
	return json.Marshal(&struct {
		*Alias
		ResumoPlaintext                string   `json:"resumo_plaintext,omitempty"`
		ResultadoSolicitacaoPlaintext  string   `json:"resultado_solicitacao_plaintext,omitempty"`
		DescricaoCompletaPlaintext     string   `json:"descricao_completa_plaintext,omitempty"`
		DocumentosNecessariosPlaintext []string `json:"documentos_necessarios_plaintext,omitempty"`
		InstrucoesSolicitantePlaintext string   `json:"instrucoes_solicitante_plaintext,omitempty"`
	}{
		Alias:                          (*Alias)(s),
		ResumoPlaintext:                utils.StripMarkdown(s.Resumo),
		ResultadoSolicitacaoPlaintext:  utils.StripMarkdown(s.ResultadoSolicitacao),
		DescricaoCompletaPlaintext:     utils.StripMarkdown(s.DescricaoCompleta),
		DocumentosNecessariosPlaintext: utils.StripMarkdownArray(s.DocumentosNecessarios),
		InstrucoesSolicitantePlaintext: utils.StripMarkdown(s.InstrucoesSolicitante),
	})
}

// PrefRioServiceRequest representa os dados de entrada para criar/atualizar um serviço
type PrefRioServiceRequest struct {
	NomeServico           string                 `json:"nome_servico" validate:"required,max=20000"`
	OrgaoGestor           []string               `json:"orgao_gestor" validate:"required,min=1"`
	Resumo                string                 `json:"resumo" validate:"required,max=20000"`
	TempoAtendimento      string                 `json:"tempo_atendimento,omitempty" validate:"max=20000"`
	CustoServico          string                 `json:"custo_servico,omitempty" validate:"max=20000"`
	ResultadoSolicitacao  string                 `json:"resultado_solicitacao,omitempty" validate:"max=20000"`
	DescricaoCompleta     string                 `json:"descricao_completa,omitempty" validate:"max=20000"`
	DocumentosNecessarios []string               `json:"documentos_necessarios"`
	InstrucoesSolicitante string                 `json:"instrucoes_solicitante" validate:"max=20000"`
	CanaisDigitais        []string               `json:"canais_digitais"`
	CanaisPresenciais     []string               `json:"canais_presenciais"`
	ServicoNaoCobre       string                 `json:"servico_nao_cobre" validate:"max=20000"`
	LegislacaoRelacionada []string               `json:"legislacao_relacionada"`
	TemaGeral             string                 `json:"tema_geral" validate:"required,max=20000"`
	SubCategoria          *string                `json:"sub_categoria,omitempty" validate:"omitempty,max=20000"`
	PublicoEspecifico     []string               `json:"publico_especifico" validate:"required,min=1"`
	FixarDestaque         bool                   `json:"fixar_destaque"`
	AwaitingApproval      bool                   `json:"awaiting_approval"`
	PublishedAt           *int64                 `json:"published_at,omitempty"`
	IsFree                *bool                  `json:"is_free,omitempty"`
	Agents                *AgentsConfig          `json:"agents,omitempty"`
	ExtraFields           map[string]interface{} `json:"extra_fields,omitempty"`
	Status                int                    `json:"status" validate:"min=0,max=1"`
	Buttons               []Button               `json:"buttons"`
}

// PrefRioServiceResponse representa a resposta de listagem de serviços
type PrefRioServiceResponse struct {
	Found    int              `json:"found"`
	OutOf    int              `json:"out_of"`
	Page     int              `json:"page"`
	Services []PrefRioService `json:"services"`
}
