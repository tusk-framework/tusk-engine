# Tusk Engine Production Hardening Design

## Goal

Tornar o servidor Go confiável para workers PHP persistentes, preservando o protocolo NDJSON e os comandos públicos existentes.

## Scope

- Confinar arquivos estáticos ao diretório público configurado.
- Aplicar limites de body, multipart e arquivos antes de alocar recursos sem limite.
- Corrigir leases, timeouts, encerramento e reinício do pool de workers.
- Validar configuração e mesclar scripts de `composer.json` e `tusk.json` conforme a documentação.
- Corrigir a imagem Docker para iniciar com o worker correto.
- Adicionar testes de regressão, concorrência e contrato NDJSON.

## Cross-repository contract

O engine envia uma requisição NDJSON com método, URI, headers, cookies, query, body, parsedBody e uploadedFiles. O worker retorna status, headers e body. O engine deve limitar e validar entradas antes do envio; o framework deve preservar o contrato e não vazar exceções.

## Design

### HTTP boundary

O caminho estático será resolvido por caminho canônico e aceito somente se permanecer dentro do diretório público. A configuração terá limites explícitos: `max_body_bytes` default 10 MiB, `max_upload_bytes` default 10 MiB por arquivo e `max_upload_files` default 20. O engine rejeitará requests acima desses limites com HTTP 413, sem ler o corpo inteiro para a memória.

### Worker pool

Cada worker terá uma única lease. Um worker só volta à fila se continuar vivo e a operação de resposta tiver terminado. A conclusão de `Wait`, timeout, kill e restart será coordenada por estado protegido; workers mortos serão removidos da fila antes do restart. O supervisor não criará novos workers após cancelamento e registrará falhas de restart.

### Configuration and packaging

`tusk.json` manterá prioridade sobre campos conflitantes, mas scripts serão mesclados de forma determinística, com os scripts de `tusk.json` vencendo em conflitos. Caminhos relativos serão resolvidos em relação ao project root. A imagem Docker copiará o worker para `/app/worker.php`, caminho padrão procurado pelo engine, e terá um smoke test de inicialização.

### Observability

Métricas continuarão disponíveis, mas o comportamento de exposição será documentado e não deverá ampliar a superfície pública sem configuração explícita. Logs de erro não devem incluir payloads completos ou segredos.

## Verification

- `go test ./...` e `go test -race ./...`.
- `go vet ./...` e `go build ./...`.
- Testes de path traversal, limites, headers, timeout, restart, shutdown e configuração.
- Teste de contrato com worker PHP mínimo quando PHP estiver disponível.

## Non-goals

- Trocar NDJSON por FastCGI ou outro transporte.
- Implementar um gerenciador completo de PHP sidecar nesta entrega.
- Reescrever a CLI inteira.
