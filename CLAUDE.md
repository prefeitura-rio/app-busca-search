# CLAUDE.md

PROIBIDO FAZER COMMIT SOZINHO!!

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is **app-busca-search**, a hybrid search API for Prefeitura do Rio de Janeiro that combines textual and vector search using Typesense and Google Gemini embeddings. It provides search capabilities across multiple service collections (1746, Carioca Digital) with relevance-based ranking and category filtering.

## Build and Development Commands

### Using Just (recommended)
```bash
# Run the application
just run

# Generate/update Swagger documentation
just swagger

# Run with swagger generation
just start

# Build binary
just build

# Run tests
just test

# Tidy dependencies
just tidy
```

### Using Go directly
```bash
# Run application
go run ./cmd/api

# Generate Swagger docs
go install github.com/swaggo/swag/cmd/swag@latest
$(go env GOPATH)/bin/swag init -g cmd/api/main.go --parseDependency --parseInternal

# Build
go build -o app-busca-search ./cmd/api

# Run tests
go test ./...
```

### Docker
```bash
# Build image (multi-stage with Swagger generation)
docker build -t app-busca-search .

# Run container
docker run -p 8080:8080 --env-file .env app-busca-search
```

## Architecture Overview

### Core Search Flow
1. **Hybrid Search**: Combines textual search (Typesense) with vector search (Google Gemini embeddings)
   - Text matching on `search_content`, `titulo`, `descricao` fields
   - Vector similarity on 768-dimensional embeddings
   - Alpha parameter (0.3) balances text vs vector scoring

2. **Relevance Ranking**: Documents are scored based on:
   - Text match score from Typesense
   - Vector distance (lower is better)
   - Service volumetry data from CSV files (1746 calls, Carioca Digital usage)

3. **Filtering System**: Removes duplicate/similar services between collections using CSV-based exclusion list

### Key Components

**Typesense Client** (`internal/typesense/client.go`)
- Manages all Typesense operations and embedding generation
- Handles multi-collection searches with custom relevance sorting
- Implements category-based search with volumetry-based ranking
- Provides CRUD operations for prefrio_services_base collection

**Search Services**
- `RelevanciaService`: Loads volumetry data from CSV files, updates periodically
- `FilterService`: Manages service exclusion list to prevent duplicates across collections

**Authentication & Authorization**
- JWT-based authentication extracting user data (name, CPF, email, ID)
- No signature validation - assumes external validation (Istio/API Gateway)
- Role extraction for audit logs only (not enforced in API)
- Admin routes require valid JWT but don't enforce role-based permissions

### Data Flow

```
Client Request
    ↓
Gin Router (routes.go)
    ↓
Handler (busca.go / admin.go)
    ↓
Typesense Client
    ↓
[Gemini API] → Embedding Generation (768-dim vectors)
    ↓
[Typesense] → Search Execution
    ↓
[RelevanciaService] → Apply Volumetry Scoring
    ↓
[FilterService] → Remove Duplicates
    ↓
Response with Sorted Results
```

## Important Implementation Details

### Embedding Generation
- Uses Google Gemini `text-embedding-004` model
- Generates 768-dimensional float32 vectors
- Falls back to text-only search if Gemini unavailable
- Embeddings stored in Typesense with `embedding` field (excluded from responses)

### Multi-Collection Search Pattern
The API searches across multiple collections (e.g., "1746,carioca-digital") and:
1. Executes parallel searches via Typesense MultiSearch API
2. Combines all results into single array
3. Sorts by text_match (desc) then vector_distance (asc)
4. Applies FilterService to remove duplicates
5. Applies RelevanciaService scoring
6. Re-sorts by relevance
7. Manually paginates the combined results

### Category Search with Relevance
Categories are ranked by total volumetry of their services:
- Fetches all services in category (paginated at 250/page internally)
- Sums volumetry scores from RelevanciaService
- Orders categories by total relevance
- Returns category metadata with service counts

### Admin CRUD Operations
Located in `internal/api/handlers/admin.go`:
- Creates services with auto-generated embeddings
- Updates services and regenerates search_content + embeddings
- Stores author from JWT user name
- Manages timestamps (created_at, last_update)
- Handles status field (0=draft, 1=published)

## Environment Configuration

Required environment variables (loaded from `.env`):
```bash
# Typesense
TYPESENSE_HOST=localhost
TYPESENSE_PORT=8108
TYPESENSE_API_KEY=your-api-key
TYPESENSE_PROTOCOL=http

# Server
SERVER_PORT=8080

# Google Gemini
GEMINI_API_KEY=your-gemini-key
GEMINI_EMBEDDING_MODEL=text-embedding-004

# Relevance Data (CSV files in data/)
RELEVANCIA_ARQUIVO_1746=data/volumetria_1746.csv
RELEVANCIA_ARQUIVO_CARIOCA_DIGITAL=data/volumetria_carioca_digital.csv
RELEVANCIA_INTERVALO_ATUALIZACAO=60  # minutes

# Filter (CSV with service IDs to exclude)
FILTER_CSV_PATH=data/servicos_similares_carioca_1746_20250702_095454.csv
```

## Project Structure Notes

- `cmd/api/main.go` - Entry point, minimal initialization
- `internal/api/routes/routes.go` - Route definitions, middleware setup
- `internal/api/handlers/` - Request handlers (busca, admin)
- `internal/typesense/client.go` - Main search logic (1200+ lines)
- `internal/services/` - RelevanciaService, FilterService
- `internal/middleware/` - JWT auth, user context extraction
- `internal/models/` - Data structures for services, categories, documents
- `internal/config/` - Environment variable loading
- `internal/utils/` - Category normalization utilities
- `internal/constants/` - Valid categories list
- `docs/` - Auto-generated Swagger documentation
- `data/` - CSV files for relevance and filtering

## Testing Notes

### Manual API Testing
```bash
# Test hybrid search
curl "http://localhost:8080/api/v1/busca-hibrida-multi?collections=1746,carioca-digital&q=certidao&page=1&per_page=10"

# Test category search
curl "http://localhost:8080/api/v1/categoria/1746,carioca-digital?categoria=documentos&page=1&per_page=10"

# Test document by ID
curl "http://localhost:8080/api/v1/documento/1746/some-doc-id"

# Test categories with relevance
curl "http://localhost:8080/api/v1/categorias-relevancia?collections=1746,carioca-digital"

# Test admin endpoints (requires JWT)
curl -H "Authorization: Bearer <jwt-token>" \
     "http://localhost:8080/api/v1/admin/services"
```

### Admin Testing Script
See `test_admin_endpoints.sh` for comprehensive admin endpoint tests

## Swagger Documentation

Access interactive API docs at: `http://localhost:8080/swagger/index.html`

Swagger files are auto-generated - always run `just swagger` or `swag init` after changing handler comments with Swagger annotations.

## JWT Authentication Notes

Current implementation (see JWT_IMPLEMENTATION.md):
- Parses JWT payload without signature validation
- Extracts: `preferred_username` (CPF), `name`, `email`, `sub` (ID)
- Extracts role from `resource_access.superapp.roles` for audit logs only
- Assumes external JWT validation (Istio, API Gateway, etc.)
- All admin endpoints require valid JWT but don't enforce role permissions

For production deployment with Istio, see RBAC_IMPLEMENTATION.md for header injection configuration.

## Common Pitfalls

1. **Swagger generation**: Must run before building Docker image or the docs will be stale
2. **Embedding failures**: If Gemini API fails, search falls back to text-only (check logs)
3. **CSV file paths**: Relative paths in .env are from working directory, not executable location
4. **Collection names**: Must match exactly in Typesense (case-sensitive)
5. **Category normalization**: Categories are normalized (lowercased, accents removed) before search - use `utils.NormalizarCategoria()` and `utils.DesnormalizarCategoria()`
6. **Pagination limits**: Typesense max per_page is 250, but API exposes max 100 to clients
7. **Multi-collection pagination**: Done manually after combining results, not at Typesense level

## Deployment

Kubernetes manifests in `k8s/staging/` and `k8s/prod/`

The application:
- Listens on port 8080
- Requires connectivity to Typesense instance
- Needs Gemini API key for embedding generation
- Loads CSV data files from `./data/` directory at startup
- Updates relevance data periodically based on RELEVANCIA_INTERVALO_ATUALIZACAO
