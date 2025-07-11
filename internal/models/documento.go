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