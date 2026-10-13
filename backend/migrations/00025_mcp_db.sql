-- +goose Up
ALTER TABLE tribe_mcp_servers DROP CONSTRAINT tribe_mcp_servers_transport_check;
ALTER TABLE tribe_mcp_servers ADD CONSTRAINT tribe_mcp_servers_transport_check
  CHECK (transport IN ('http', 'stdio', 'postgres', 'sqlserver'));
ALTER TABLE tribe_mcp_servers ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
DELETE FROM tribe_mcp_servers WHERE transport IN ('postgres', 'sqlserver');
ALTER TABLE tribe_mcp_servers DROP COLUMN config_json;
ALTER TABLE tribe_mcp_servers DROP CONSTRAINT tribe_mcp_servers_transport_check;
ALTER TABLE tribe_mcp_servers ADD CONSTRAINT tribe_mcp_servers_transport_check
  CHECK (transport IN ('http', 'stdio'));
