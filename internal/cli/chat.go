package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

const (
	maxChatToolRounds = 16
	maxChatMessages   = 40
)

var documentChatTool = vlm.AgentTool{
	Name:        "convert_document",
	Description: "Convert a user-provided file, directory, or HTTP URL into Markdown with doc7. Use only when the user explicitly asks to process a document and supplies the input reference.",
	Parameters: []byte(`{
  "type": "object",
  "properties": {
    "input": {
      "type": "string",
      "description": "Exact file path, directory path, or HTTP URL explicitly provided by the user"
    },
    "document_id": {
      "type": "string",
      "description": "Confirmed document ID returned by a read-only file tool"
    },
    "instruction": {
      "type": "string",
      "description": "Optional user requirements for the Markdown output"
    }
  },
  "additionalProperties": false
}`),
}

func chatToolDefinitions() []vlm.AgentTool {
	tools := []vlm.AgentTool{documentChatTool}
	tools = append(tools, fileChatTools...)
	tools = append(tools, inputSecretChatTool)
	return append(tools, configurationChatTools...)
}

type chatAgent struct {
	command                  *cobra.Command
	config                   config.AppConfig
	readFlags                readFlags
	messages                 []vlm.AgentMessage
	userInput                []string
	interactions             map[string]chatUserInteraction
	pendingConfig            *pendingConfigChange
	authorizedRoots          []string
	documents                map[string]*chatDocumentReference
	lastInteractionSignature string
	lastInteractionResult    string
}

func newChatCommand() *cobra.Command {
	flags := defaultReadFlags()
	command := &cobra.Command{
		Use:   "chat [message]",
		Short: translate("chat.short"),
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeChat(cmd, args, flags)
		},
	}
	flags.bind(command)
	return command
}

func executeChat(cmd *cobra.Command, args []string, flags readFlags) error {
	localConfig := flags.config()
	applyChangedNumericFlags(cmd, &localConfig)
	cfg, err := loadConfig(localConfig, false)
	if err != nil {
		return err
	}
	if cfg.JSONOutput {
		return vlm.NewError(vlm.ConfigError, translate("chat.json_unsupported"), false, nil)
	}
	cfg, err = ensureChatConfig(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	agent := newChatAgent(cmd, cfg, flags)
	if len(args) > 0 {
		return agent.respond(strings.TrimSpace(strings.Join(args, " ")))
	}
	if !stdinIsTerminal() {
		return vlm.NewError(vlm.ConfigError, translate("chat.non_interactive"), false, nil)
	}
	return agent.runInteractive(os.Stdin)
}

func newChatAgent(command *cobra.Command, cfg config.AppConfig, flags readFlags) *chatAgent {
	return &chatAgent{
		command:         command,
		config:          cfg,
		readFlags:       flags,
		authorizedRoots: defaultChatRoots(),
		documents:       make(map[string]*chatDocumentReference),
		messages: []vlm.AgentMessage{
			{Role: "system", Content: chatSystemPrompt()},
		},
		interactions: make(map[string]chatUserInteraction),
	}
}

func (a *chatAgent) runInteractive(input *os.File) error {
	writeText("%s", translate("chat.started", a.config.Model))
	scanner := bufio.NewScanner(input)
	for {
		fmt.Fprint(os.Stdout, "doc7> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout)
			return nil
		}
		message := strings.TrimSpace(scanner.Text())
		if message == "" {
			continue
		}
		if isChatExit(message) {
			return nil
		}
		if err := a.respond(message); err != nil {
			return err
		}
	}
}

func (a *chatAgent) respond(message string) error {
	if message == "" {
		return vlm.NewError(vlm.ConfigError, translate("chat.empty_message"), false, nil)
	}
	a.userInput = append(a.userInput, message)
	a.lastInteractionSignature = ""
	a.lastInteractionResult = ""
	a.messages = append(a.messages, vlm.AgentMessage{Role: "user", Content: message})
	for round := 0; round < maxChatToolRounds; round++ {
		response, err := vlm.CompleteAgentChatOpenAICompatible(
			a.command.Context(),
			readVLMConfig(a.config),
			a.messages,
			chatToolDefinitions(),
			nil,
		)
		if err != nil {
			return err
		}
		a.messages = append(a.messages, vlm.AgentMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		if len(response.ToolCalls) == 0 {
			writeText("%s", strings.TrimSpace(response.Content))
			a.trimHistory()
			return nil
		}
		terminalStatus := chatToolContinue
		for _, call := range response.ToolCalls {
			execution := a.executeTool(call)
			if execution.status != chatToolContinue {
				terminalStatus = execution.status
			}
			a.messages = append(a.messages, vlm.AgentMessage{
				Role:       "tool",
				Content:    execution.result,
				ToolCallID: call.ID,
			})
		}
		if terminalStatus == chatToolDocumentCompleted {
			writeText("%s", translate("chat.conversion_complete"))
			a.trimHistory()
			return nil
		}
		if terminalStatus == chatToolConfigurationApplied {
			writeText("%s", translate("chat.configuration_applied"))
			a.trimHistory()
			return nil
		}
	}
	return vlm.NewError(vlm.ModelError, translate("chat.tool_limit"), false, nil)
}

func (a *chatAgent) trimHistory() {
	if len(a.messages) <= maxChatMessages {
		return
	}
	start := len(a.messages) - maxChatMessages + 1
	for start < len(a.messages) && a.messages[start].Role != "user" {
		start++
	}
	recent := append([]vlm.AgentMessage(nil), a.messages[start:]...)
	a.messages = append([]vlm.AgentMessage{{Role: "system", Content: chatSystemPrompt()}}, recent...)
}

func chatSystemPrompt() string {
	return `You are doc7, a concise assistant for document-to-Markdown work.
Reply to ordinary conversation normally in the user's language.
When the user gives an exact file path, directory path, or HTTP URL, call convert_document. When the user gives an incomplete filename or vague location, use run_readonly_command to inspect authorized directories, then use ask_user to let the user select a candidate. Put the candidate's exact document_id in the option.document_id field. After selection, call convert_document with that document_id instead of asking again.
Use only the structured read-only commands. Never construct shell syntax, pipes, redirects, substitutions, scripts, or write operations. Use authorize_directory when the user wants to search outside the currently authorized roots.
Use pwd, ls, find, file, stat, wc, and realpath for discovery. head and tail reveal file content to the model, so the local tool asks the user for confirmation before reading it. Call the tool once and use its result.
When the user asks about doc7 configuration, use get_configuration first. Use discover_local_models to find local runtimes and verify_model_configuration to test a candidate model with a real image.
For configuration changes, first call set_configuration without a confirmation_id to create a dry-run proposal. Then call ask_user with a clear question and options whose IDs are exactly "confirm" and "cancel". Only after the user selects confirm may you call set_configuration again with the returned interaction_id as confirmation_id.
When an authenticated endpoint needs an API key, call input_secret before applying a pending model configuration. The local tool confirms the target and asks the user for the secret with input hidden. Call it once and use its result. The tool may report the input length and storage source, but the secret content is never visible to you.
Use ask_user for consequential choices such as selecting among local models. Do not treat a tool call or your own assumption as user confirmation.
After any tool result, continue the requested workflow. If another tool is required, call it immediately instead of describing a future call. Never stop after saying that you will call a tool.
After ask_user returns, use selected_options as the exact values for the next tool. Do not ask the same question again unless the selection is invalid.
If ask_user reports cancelled=true, stop the requested action and tell the user it was cancelled. Do not ask the same question again.
For a read-only discovery or verification request, report the tool result when it is complete. Do not ask whether to apply, use, or save anything unless the user requested a change.
Never ask the user to paste API keys, tokens, passwords, authorization headers, or other secrets into chat or ask_user. Use input_secret. Never repeat or infer a secret from its length.
Never invent a path or broaden an authorization root. Never claim a conversion succeeded before the tool reports success.
Use only the provided tools.`
}

func isChatExit(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "exit", "quit", "/exit", "/quit":
		return true
	default:
		return false
	}
}
