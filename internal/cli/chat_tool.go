package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/extract"
	readinput "github.com/magicrew/doc7/internal/read"
	"github.com/magicrew/doc7/internal/remoteinput"
	"github.com/magicrew/doc7/internal/vlm"
)

type convertDocumentArguments struct {
	Input       string `json:"input"`
	DocumentID  string `json:"document_id"`
	Instruction string `json:"instruction"`
}

type chatToolResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	Markdown  string `json:"markdown,omitempty"`
	Artifacts string `json:"artifacts,omitempty"`
}

type chatToolStatus string

const (
	chatToolContinue             chatToolStatus = "continue"
	chatToolDocumentCompleted    chatToolStatus = "document_completed"
	chatToolConfigurationApplied chatToolStatus = "configuration_applied"
)

type chatToolExecution struct {
	result string
	status chatToolStatus
}

func (a *chatAgent) executeTool(call vlm.AgentToolCall) chatToolExecution {
	if call.Function.Name == "ask_user" || isConfigurationTool(call.Function.Name) {
		return a.executeConfigurationTool(call)
	}
	if isFileTool(call.Function.Name) {
		return a.executeFileTool(call)
	}
	if call.Function.Name == inputSecretChatTool.Name {
		return a.executeInputSecret(call.Function.Arguments)
	}
	if call.Function.Name != "convert_document" {
		return chatToolExecution{result: encodeChatToolResult(false, fmt.Sprintf("unsupported tool: %s", call.Function.Name)), status: chatToolContinue}
	}
	var arguments convertDocumentArguments
	if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid convert_document arguments: "+err.Error()), status: chatToolContinue}
	}
	arguments.Input = strings.TrimSpace(arguments.Input)
	arguments.DocumentID = strings.TrimSpace(arguments.DocumentID)
	if arguments.DocumentID != "" {
		document, ok := a.documents[arguments.DocumentID]
		if !ok || !document.Confirmed {
			return chatToolExecution{result: encodeChatToolResult(false, "the document ID was not selected by the user"), status: chatToolContinue}
		}
		arguments.Input = document.Path
	} else {
		if arguments.Input == "" {
			return chatToolExecution{result: encodeChatToolResult(false, "convert_document requires input or document_id"), status: chatToolContinue}
		}
		if !a.userProvidedInput(arguments.Input) {
			return chatToolExecution{result: encodeChatToolResult(false, "the input was not explicitly provided by the user"), status: chatToolContinue}
		}
	}
	if !isChatDocumentInput(arguments.Input) {
		return chatToolExecution{result: encodeChatToolResult(false, "the input does not exist or is not an HTTP URL"), status: chatToolContinue}
	}

	flags := a.readFlags
	var readResult readinput.Result
	flags.result = &readResult
	cleanup := func() {}
	if strings.TrimSpace(arguments.Instruction) != "" {
		promptPath, removePrompt, err := chatPromptFile(arguments.Input, arguments.Instruction)
		if err != nil {
			return chatToolExecution{result: encodeChatToolResult(false, "failed to prepare conversion instruction: "+err.Error()), status: chatToolContinue}
		}
		flags.PromptFile = promptPath
		flags.Prompt = ""
		cleanup = removePrompt
	}
	defer cleanup()
	if err := executeRead(a.command, arguments.Input, &flags); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	markdown := ""
	if readResult.Document != nil {
		markdown = readResult.Document.MergedMarkdown
	}
	return chatToolExecution{result: encodeChatToolSuccess(markdown, readResult.OutputDir), status: chatToolDocumentCompleted}
}

func (a *chatAgent) userProvidedInput(input string) bool {
	for _, message := range a.userInput {
		if strings.Contains(message, input) {
			return true
		}
	}
	return false
}

func isChatDocumentInput(value string) bool {
	if remoteinput.IsHTTPURL(value) {
		return true
	}
	_, err := os.Stat(value)
	return err == nil
}

func encodeChatToolResult(ok bool, message string) string {
	data, err := json.Marshal(chatToolResult{OK: ok, Message: message})
	if err != nil {
		return `{"ok":false,"message":"failed to encode tool result"}`
	}
	return string(data)
}

func encodeChatToolSuccess(markdown string, artifacts string) string {
	data, err := json.Marshal(chatToolResult{
		OK:        true,
		Message:   "The document conversion completed. These paths come from the doc7 conversion result.",
		Markdown:  markdown,
		Artifacts: artifacts,
	})
	if err != nil {
		return `{"ok":true,"message":"document conversion completed"}`
	}
	return string(data)
}

func chatPromptFile(inputPath string, instruction string) (string, func(), error) {
	kind := detect.KindPDF
	if !remoteinput.IsHTTPURL(inputPath) {
		if input, err := detect.Detect(inputPath); err == nil {
			kind = input.Kind
		}
	}
	basePrompt, err := extract.PromptForInput("auto", "", kind)
	if err != nil {
		return "", nil, err
	}
	file, err := os.CreateTemp("", "doc7-chat-*.txt")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	prompt := basePrompt + "\n\n" + translate("chat.prompt_prefix", instruction) + "\n"
	if _, err := file.WriteString(prompt); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return filepath.Clean(file.Name()), cleanup, nil
}
