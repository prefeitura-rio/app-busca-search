package synonyms

import "strings"

// SynonymGroup representa um grupo de sinônimos
type SynonymGroup struct {
	Root     string   // termo principal
	Synonyms []string // sinônimos
}

// DefaultSynonyms contém sinônimos padrão para serviços públicos
var DefaultSynonyms = []SynonymGroup{
	// Ações comuns
	{Root: "tirar", Synonyms: []string{"emitir", "solicitar", "obter", "conseguir", "pedir", "requerer"}},
	{Root: "pagar", Synonyms: []string{"quitar", "liquidar", "efetuar pagamento", "pagar conta", "saldar"}},
	{Root: "renovar", Synonyms: []string{"atualizar", "refazer", "revalidar", "prolongar"}},
	{Root: "consultar", Synonyms: []string{"verificar", "checar", "pesquisar", "buscar informação"}},
	{Root: "agendar", Synonyms: []string{"marcar", "reservar", "programar"}},
	{Root: "cancelar", Synonyms: []string{"anular", "desistir", "revogar"}},

	// Documentos
	{Root: "documento", Synonyms: []string{"certidão", "comprovante", "atestado", "declaração", "certificado"}},
	{Root: "carteira", Synonyms: []string{"carteirinha", "crachá", "identificação", "credencial"}},
	{Root: "certidão", Synonyms: []string{"documento", "atestado", "comprovante", "declaração"}},
	{Root: "comprovante", Synonyms: []string{"recibo", "comprovação", "prova"}},

	// Impostos e taxas
	{Root: "iptu", Synonyms: []string{"imposto predial", "imposto territorial urbano", "imposto do imóvel", "imposto da casa"}},
	{Root: "iss", Synonyms: []string{"imposto sobre serviços", "imposto de serviço", "iss qn"}},
	{Root: "itbi", Synonyms: []string{"imposto transmissão", "imposto compra imóvel", "imposto transferência"}},
	{Root: "taxa", Synonyms: []string{"tarifa", "tributo", "cobrança"}},
	{Root: "multa", Synonyms: []string{"penalidade", "infração", "auto de infração"}},

	// Serviços comuns
	{Root: "segunda via", Synonyms: []string{"2a via", "2ª via", "cópia", "duplicata", "nova via"}},
	{Root: "alvará", Synonyms: []string{"licença", "autorização", "permissão", "licenciamento"}},
	{Root: "matrícula", Synonyms: []string{"inscrição", "registro", "cadastro", "cadastramento"}},
	{Root: "habilitação", Synonyms: []string{"cnh", "carteira de motorista", "licença para dirigir"}},
	{Root: "identidade", Synonyms: []string{"rg", "carteira de identidade", "documento de identidade"}},

	// Locais
	{Root: "prefeitura", Synonyms: []string{"municipio", "governo municipal", "pmrj", "pcrj"}},
	{Root: "posto", Synonyms: []string{"unidade", "agência", "central de atendimento"}},

	// Público
	{Root: "idoso", Synonyms: []string{"terceira idade", "melhor idade", "60+", "aposentado", "pessoa idosa"}},
	{Root: "deficiente", Synonyms: []string{"pcd", "pessoa com deficiência", "portador de deficiência", "necessidades especiais"}},
	{Root: "criança", Synonyms: []string{"menor", "infantil", "crianças"}},
	{Root: "estudante", Synonyms: []string{"aluno", "universitário", "escolar"}},

	// Saúde
	{Root: "vacina", Synonyms: []string{"vacinação", "imunização", "dose"}},
	{Root: "consulta", Synonyms: []string{"atendimento médico", "consulta médica", "agendamento saúde"}},
	{Root: "exame", Synonyms: []string{"teste", "análise", "diagnóstico"}},

	// Educação
	{Root: "escola", Synonyms: []string{"colégio", "instituição de ensino", "unidade escolar"}},
	{Root: "creche", Synonyms: []string{"educação infantil", "pré-escola", "berçário"}},
	{Root: "vaga", Synonyms: []string{"matrícula", "inscrição", "lugar"}},

	// Transporte
	{Root: "bilhete único", Synonyms: []string{"bu", "cartão de transporte", "passe"}},
	{Root: "estacionamento", Synonyms: []string{"zona azul", "parquímetro", "vaga rotativa"}},

	// Habitação
	{Root: "imóvel", Synonyms: []string{"propriedade", "terreno", "casa", "apartamento", "residência"}},
	{Root: "aluguel", Synonyms: []string{"locação", "arrendamento"}},

	// Urbanismo
	{Root: "obra", Synonyms: []string{"construção", "reforma", "edificação"}},
	{Root: "habite-se", Synonyms: []string{"auto de conclusão", "habite se", "habitese"}},

	// Veículos
	{Root: "carro", Synonyms: []string{"veículo", "automóvel", "auto", "viatura"}},
	{Root: "moto", Synonyms: []string{"motocicleta", "motoca", "motinho"}},
	{Root: "licenciamento", Synonyms: []string{"licença veicular", "documento do carro", "crlv", "licença do veículo"}},

	// Meio ambiente
	{Root: "árvore", Synonyms: []string{"poda", "corte de árvore", "arborização", "vegetação"}},
	{Root: "lixo", Synonyms: []string{"coleta", "resíduos", "entulho", "descarte"}},
	{Root: "animal", Synonyms: []string{"pet", "cachorro", "gato", "bicho", "fauna"}},

	// Assistência social
	{Root: "benefício", Synonyms: []string{"auxílio", "ajuda", "assistência", "bolsa"}},
	{Root: "cadastro único", Synonyms: []string{"cadunico", "cadúnico", "cad único", "cadastro social"}},
	{Root: "cras", Synonyms: []string{"centro de referência", "assistência social", "serviço social"}},

	// Segurança
	{Root: "ocorrência", Synonyms: []string{"bo", "boletim de ocorrência", "registro policial", "queixa"}},
	{Root: "delegacia", Synonyms: []string{"dp", "distrito policial", "polícia"}},

	// Trabalhador
	{Root: "emprego", Synonyms: []string{"trabalho", "vaga de emprego", "oportunidade", "colocação"}},
	{Root: "ctps", Synonyms: []string{"carteira de trabalho", "carteira profissional"}},
	{Root: "mei", Synonyms: []string{"microempreendedor", "micro empreendedor individual", "cnpj mei"}},

	// Eventos e cultura
	{Root: "evento", Synonyms: []string{"show", "festa", "apresentação", "espetáculo"}},
	{Root: "museu", Synonyms: []string{"exposição", "galeria", "centro cultural"}},

	// Reclamações
	{Root: "reclamação", Synonyms: []string{"denúncia", "ouvidoria", "queixa", "reclamar"}},
	{Root: "problema", Synonyms: []string{"defeito", "falha", "irregularidade", "erro"}},

	// Parcelamento
	{Root: "parcelamento", Synonyms: []string{"parcelar", "dividir", "parcelas", "refinanciamento"}},
	{Root: "dívida", Synonyms: []string{"débito", "pendência", "inadimplência", "devendo"}},
}

// GetAllSynonyms retorna todos os sinônimos como mapa
func GetAllSynonyms() map[string][]string {
	result := make(map[string][]string)
	for _, group := range DefaultSynonyms {
		result[group.Root] = group.Synonyms
	}
	return result
}

// FindSynonyms busca sinônimos para um termo (token único)
func FindSynonyms(term string) []string {
	for _, group := range DefaultSynonyms {
		if group.Root == term {
			return group.Synonyms
		}
		for _, syn := range group.Synonyms {
			if syn == term {
				// Retorna root + outros sinônimos
				result := []string{group.Root}
				for _, s := range group.Synonyms {
					if s != term {
						result = append(result, s)
					}
				}
				return result
			}
		}
	}
	return nil
}

// FindSynonymsByPhrase busca sinônimos por frase completa (case-insensitive)
// Isso permite expansão bidirecional: "imposto predial" -> "iptu" e vice-versa
func FindSynonymsByPhrase(phrase string) []string {
	normalized := strings.ToLower(strings.TrimSpace(phrase))
	if normalized == "" {
		return nil
	}

	for _, group := range DefaultSynonyms {
		// Check if phrase matches root (case-insensitive)
		if strings.ToLower(group.Root) == normalized {
			return group.Synonyms
		}
		// Check if phrase matches any synonym (case-insensitive)
		for _, syn := range group.Synonyms {
			if strings.ToLower(syn) == normalized {
				// Retorna root + outros sinônimos (exceto o que deu match)
				result := []string{group.Root}
				for _, s := range group.Synonyms {
					if strings.ToLower(s) != normalized {
						result = append(result, s)
					}
				}
				return result
			}
		}
	}
	return nil
}
