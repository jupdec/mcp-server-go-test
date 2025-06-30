package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	"github.com/google/uuid"
)

func main() {
	ctx := context.Background()

	// Load AWS config from environment or shared config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	client := bedrockagentruntime.NewFromConfig(cfg)

	input := &bedrockagentruntime.InvokeInlineAgentInput{
		FoundationModel: aws.String("us.anthropic.claude-3-5-sonnet-20241022-v2:0"),
		Instruction:     aws.String("You are a friendly assistant for resolving user queries"),
		AgentName:       aws.String("SampleAgent"),
		InputText:       aws.String("Convert 11am from NYC time to London time"),
		SessionId:       aws.String(uuid.NewString()), // <-- Required!
		EnableTrace:     aws.Bool(true),
	}

	// Call the API
	output, err := client.InvokeInlineAgent(ctx, input)
	if err != nil {
		log.Fatalf("InvokeInlineAgent failed: %v", err)
	}
	defer output.GetStream().Close()

	for event := range output.GetStream().Events() {
		switch v := event.(type) {
		case *types.InlineAgentResponseStreamMemberChunk:
			fmt.Printf("Agent response chunk: %s\n", string(v.Value.Bytes))
		case *types.InlineAgentResponseStreamMemberTrace:
			fmt.Printf("Trace event: %+v\n", v.Value)
		default:
			fmt.Printf("Unknown event: %#v\n", event)
		}
	}

}
