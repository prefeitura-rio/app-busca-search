package synonyms

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
}

// GetAllSynonyms retorna todos os sinônimos como mapa
func GetAllSynonyms() map[string][]string {
	result := make(map[string][]string)
	for _, group := range DefaultSynonyms {
		result[group.Root] = group.Synonyms
	}
	return result
}

// FindSynonyms busca sinônimos para um termo
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
