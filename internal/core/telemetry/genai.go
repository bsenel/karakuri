package telemetry

// OpenTelemetry GenAI semantic-convention keys. They live here rather than in
// the OTel adapter so code outside internal/platform can name an attribute
// without importing a vendor SDK, and so every emitter spells it the same way.
const (
	GenAIOperationName     = "gen_ai.operation.name"
	GenAIProviderName      = "gen_ai.provider.name"
	GenAIRequestModel      = "gen_ai.request.model"
	GenAIUsageInputTokens  = "gen_ai.usage.input_tokens"
	GenAIUsageOutputTokens = "gen_ai.usage.output_tokens"
	GenAIAgentName         = "gen_ai.agent.name"
	GenAIToolName          = "gen_ai.tool.name"
	GenAITokenType         = "gen_ai.token.type"
)

// Values for GenAIOperationName.
const (
	OpInvokeAgent = "invoke_agent"
	OpChat        = "chat"
	OpExecuteTool = "execute_tool"
)
