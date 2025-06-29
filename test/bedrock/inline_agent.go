package bedrock

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
)

func InvokeAgent(cfg aws.Config, agentId string, inputText string) error {
    client := bedrockagentruntime.NewFromConfig(cfg)

    _, err := client.InvokeInlineAgent(context.TODO(), &bedrockagentruntime.InvokeInlineAgentInput{
        AgentId: &agentId,
        Messages: []types.Message{
            {
                Role:    "user",
                Content: &types.MessageContent{Text: &inputText},
            },
        },
        EnableTrace: true,
    })
    return err
}
