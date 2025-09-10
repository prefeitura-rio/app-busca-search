package models

// EtapaDetalhada representa uma etapa detalhada do serviço
type EtapaDetalhada struct {
	Descricao        string `json:"descricao"`
	Ordem            int64  `json:"ordem"`
	TemLink          bool   `json:"tem_link"`
	LinkSolicitacao  string `json:"link_solicitacao,omitempty"`
	EmManutencao     bool   `json:"em_manutencao"`
}

// DocumentoDetalhado representa um documento detalhado necessário
type DocumentoDetalhado struct {
	Descricao       string                         `json:"descricao"`
	Ordem           int64                          `json:"ordem"`
	TemURL          bool                           `json:"tem_url"`
	URL             string                         `json:"url,omitempty"`
	Obrigatorio     bool                           `json:"obrigatorio"`
	PermiteUpload   bool                           `json:"permite_upload"`
	UploadInfo      *DocumentoUploadInfo           `json:"upload_info,omitempty"`
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
	ID                        string                `json:"id,omitempty"`
	Titulo                    string                `json:"titulo"`
	Descricao                 string                `json:"descricao"`
	URL                       string                `json:"url"`
	Collection                string                `json:"collection"`
	Category                  string                `json:"category"`
	UltimaAtualizacao         string                `json:"ultima_atualizacao"`
	Slug                      string                `json:"slug"`
	Status                    string                `json:"status"`
	Tipo                      string                `json:"tipo"`
	OrgaoGestor               string                `json:"orgao_gestor"`
	NomeGestor                *string               `json:"nome_gestor,omitempty"`
	InformacoesComplementares *string               `json:"informacoes_complementares,omitempty"`
	EsteServicoNaoCobre       *string               `json:"este_servico_nao_cobre,omitempty"`
	ProcedimentosEspeciais    *string               `json:"procedimentos_especiais,omitempty"`
	LinkAcesso                *string               `json:"link_acesso,omitempty"`
	DisponiveApp              bool                  `json:"disponivel_app"`
	AppAndroid                *string               `json:"app_android,omitempty"`
	AppIOS                    *string               `json:"app_ios,omitempty"`
	AtendimentoPresencial     bool                  `json:"atendimento_presencial"`
	LocalPresencial           *string               `json:"local_presencial,omitempty"`
	EmManutencao              bool                  `json:"em_manutencao"`
	Etapas                    []string              `json:"etapas,omitempty"`
	EtapasDetalhadas          []EtapaDetalhada      `json:"etapas_detalhadas,omitempty"`
	DocumentosNecessarios     []string              `json:"documentos_necessarios,omitempty"`
	DocumentosDetalhados      []DocumentoDetalhado  `json:"documentos_detalhados,omitempty"`
	PermiteUpload             bool                  `json:"permite_upload"`
	PrazoEsperado             *string               `json:"prazo_esperado,omitempty"`
	CustoDoServico            string                `json:"custo_do_servico"`
	ValorASerPago             *string               `json:"valor_a_ser_pago,omitempty"`
	Gratuito                  bool                  `json:"gratuito"`
	Temas                     []string              `json:"temas"`
	PublicoAlvo               []string              `json:"publico_alvo"`
	Atividades                []string              `json:"atividades"`
	Sistema                   string                `json:"sistema"`
	TemLegislacao             bool                  `json:"tem_legislacao"`
	Legislacao                []string              `json:"legislacao,omitempty"`
	SearchContent             string                `json:"search_content"`
	PalavrasChave             []string              `json:"palavras_chave,omitempty"`
	Destaque                  bool                  `json:"destaque"`
	ScoreCompletude           *float64              `json:"score_completude,omitempty"`
	
	// Campos de embedding (geralmente não retornados nas consultas)
	Embedding                 []float64             `json:"embedding,omitempty"`
}

// DocumentoResumo representa um documento com apenas título e ID para busca por categoria
type DocumentoResumo struct {
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
}

// BuscaCategoriaResponse representa a resposta da busca por categoria
type BuscaCategoriaResponse struct {
	Found     int               `json:"found"`
	OutOf     int               `json:"out_of"`
	Page      int               `json:"page"`
	Documentos []DocumentoResumo `json:"documentos"`
}

// PrefRioService representa um serviço da collection prefrio_services_base
type PrefRioService struct {
	ID                        string   `json:"id,omitempty" typesense:"id,optional"`
	NomeServico               string   `json:"nome_servico" validate:"required,max=255" typesense:"nome_servico"`
	OrgaoGestor               []string `json:"orgao_gestor" validate:"required,min=1" typesense:"orgao_gestor"`
	Resumo                    string   `json:"resumo" validate:"required,max=350" typesense:"resumo"`
	TempoAtendimento          string   `json:"tempo_atendimento" validate:"required" typesense:"tempo_atendimento"`
	CustoServico              string   `json:"custo_servico" validate:"required" typesense:"custo_servico"`
	ResultadoSolicitacao      string   `json:"resultado_solicitacao" validate:"required" typesense:"resultado_solicitacao"`
	DescricaoCompleta         string   `json:"descricao_completa" validate:"required" typesense:"descricao_completa"`
	Autor                     string   `json:"autor" validate:"required" typesense:"autor"`
	DocumentosNecessarios     []string `json:"documentos_necessarios,omitempty" typesense:"documentos_necessarios,optional"`
	InstrucoesSolicitante     string   `json:"instrucoes_solicitante,omitempty" typesense:"instrucoes_solicitante,optional"`
	CanaisDigitais            []string `json:"canais_digitais,omitempty" typesense:"canais_digitais,optional"`
	CanaisPresenciais         []string `json:"canais_presenciais,omitempty" typesense:"canais_presenciais,optional"`
	ServicoNaoCobre           string   `json:"servico_nao_cobre,omitempty" typesense:"servico_nao_cobre,optional"`
	LegislacaoRelacionada     []string `json:"legislacao_relacionada,omitempty" typesense:"legislacao_relacionada,optional"`
	TemaGeral                 string   `json:"tema_geral" validate:"required" typesense:"tema_geral"`
	PublicoEspecifico         []string `json:"publico_especifico,omitempty" typesense:"publico_especifico,optional"`
	ObjetivoCidadao           string   `json:"objetivo_cidadao" validate:"required" typesense:"objetivo_cidadao"`
	FixarDestaque             bool     `json:"fixar_destaque" typesense:"fixar_destaque"`
	Status                    int      `json:"status" validate:"min=0,max=1" typesense:"status"` // 0=Draft, 1=Published
	CreatedAt                 int64    `json:"created_at" typesense:"created_at"`
	UpdatedAt                 int64    `json:"updated_at" typesense:"updated_at"`
	SearchContent             string   `json:"search_content" typesense:"search_content"`
	Embedding                 []float64 `json:"embedding,omitempty" typesense:"embedding,optional"`
}

// PrefRioServiceRequest representa os dados de entrada para criar/atualizar um serviço
type PrefRioServiceRequest struct {
	NomeServico               string   `json:"nome_servico" validate:"required,max=255"`
	OrgaoGestor               []string `json:"orgao_gestor" validate:"required,min=1"`
	Resumo                    string   `json:"resumo" validate:"required,max=350"`
	TempoAtendimento          string   `json:"tempo_atendimento" validate:"required"`
	CustoServico              string   `json:"custo_servico" validate:"required"`
	ResultadoSolicitacao      string   `json:"resultado_solicitacao" validate:"required"`
	DescricaoCompleta         string   `json:"descricao_completa" validate:"required"`
	DocumentosNecessarios     []string `json:"documentos_necessarios,omitempty"`
	InstrucoesSolicitante     string   `json:"instrucoes_solicitante,omitempty"`
	CanaisDigitais            []string `json:"canais_digitais,omitempty"`
	CanaisPresenciais         []string `json:"canais_presenciais,omitempty"`
	ServicoNaoCobre           string   `json:"servico_nao_cobre,omitempty"`
	LegislacaoRelacionada     []string `json:"legislacao_relacionada,omitempty"`
	TemaGeral                 string   `json:"tema_geral" validate:"required"`
	PublicoEspecifico         []string `json:"publico_especifico,omitempty"`
	ObjetivoCidadao           string   `json:"objetivo_cidadao" validate:"required"`
	FixarDestaque             bool     `json:"fixar_destaque"`
	Status                    int      `json:"status" validate:"min=0,max=1"`
}

// PrefRioServiceResponse representa a resposta de listagem de serviços
type PrefRioServiceResponse struct {
	Found     int              `json:"found"`
	OutOf     int              `json:"out_of"`
	Page      int              `json:"page"`
	Services  []PrefRioService `json:"services"`
} 