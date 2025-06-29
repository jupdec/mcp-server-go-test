package config

import "os"

type Config struct {
    MCPURL   string
    Region   string
    AgentId  string
    ModelArn string
}

func Load() *Config {
    return &Config{
        MCPURL:   os.Getenv("MCP_URL"),
        Region:   os.Getenv("AWS_REGION"),
        AgentId:  os.Getenv("AGENT_ID"),
        ModelArn: os.Getenv("MODEL_ARN"),
    }
}
