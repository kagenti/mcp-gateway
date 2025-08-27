package config

type MCPServersConfig struct {
	Servers []*MCPServer
}

type MCPServer struct {
	Name       string
	URL        string
	ToolPrefix string
	Enabled    bool
	Hostname   string
}
