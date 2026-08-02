package cli

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/magicrew/doc7/internal/vlm"
)

var askUserChatTool = vlm.AgentTool{
	Name:        "ask_user",
	Description: "Ask the user to choose from a small set of explicit options before a consequential action. Use this for model selection and configuration confirmation.",
	Parameters: []byte(`{
  "type": "object",
  "properties": {
    "question": {"type": "string", "description": "Short question shown to the user"},
    "options": {
      "type": "array",
      "minItems": 2,
      "maxItems": 8,
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string", "description": "Stable option identifier returned to the model"},
          "label": {"type": "string", "description": "Human-readable option label"},
          "description": {"type": "string", "description": "Brief explanation of the option"},
          "document_id": {"type": "string", "description": "Document ID from a read-only file tool when this option selects a file"}
        },
        "required": ["id", "label"],
        "additionalProperties": false
      }
    },
    "allow_multiple": {"type": "boolean", "description": "Whether the user may select more than one option"}
  },
  "required": ["question", "options"],
  "additionalProperties": false
}`),
}

type chatUserOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	DocumentID  string `json:"document_id,omitempty"`
}

type askUserArguments struct {
	Question      string           `json:"question"`
	Options       []chatUserOption `json:"options"`
	AllowMultiple bool             `json:"allow_multiple"`
}

type chatUserInteraction struct {
	ID        string
	Selected  []string
	Confirmed bool
}

func (a *chatAgent) executeAskUser(raw string) chatToolExecution {
	var arguments askUserArguments
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "invalid ask_user arguments: "+err.Error()), status: chatToolContinue}
	}
	arguments.Question = strings.TrimSpace(arguments.Question)
	if arguments.Question == "" || len(arguments.Options) < 2 || len(arguments.Options) > 8 {
		return chatToolExecution{result: encodeChatToolResult(false, "ask_user requires a question and two to eight options"), status: chatToolContinue}
	}
	seen := make(map[string]bool, len(arguments.Options))
	for _, option := range arguments.Options {
		if strings.TrimSpace(option.ID) == "" || strings.TrimSpace(option.Label) == "" || seen[option.ID] {
			return chatToolExecution{result: encodeChatToolResult(false, "ask_user options require unique non-empty id and label values"), status: chatToolContinue}
		}
		seen[option.ID] = true
	}
	signature := encodeChatJSON(arguments)
	if signature == a.lastInteractionSignature && a.lastInteractionResult != "" {
		return chatToolExecution{result: a.lastInteractionResult, status: chatToolContinue}
	}
	if globals.Yes && hasConfirmationOptions(arguments.Options) {
		return a.storeInteraction(signature, arguments.Options, []string{"confirm"})
	}
	if !stdinIsTerminal() {
		return chatToolExecution{result: encodeChatToolResult(false, "ask_user requires an interactive terminal; rerun with --yes when the requested action is safe to auto-confirm"), status: chatToolContinue}
	}

	writeText("%s", arguments.Question)
	for index, option := range arguments.Options {
		if strings.TrimSpace(option.Description) == "" {
			writeText("  %d. %s", index+1, option.Label)
			continue
		}
		writeText("  %d. %s - %s", index+1, option.Label, option.Description)
	}
	if arguments.AllowMultiple {
		writeText("%s", translate("chat.ask_user.multiple"))
	} else {
		writeText("%s", translate("chat.ask_user.single"))
	}
	fmt.Fprintf(os.Stdout, "%s ", translate("chat.ask_user.prompt"))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, "failed to read user choice: "+err.Error()), status: chatToolContinue}
	}
	selected, cancelled, err := selectUserOptions(strings.TrimSpace(line), arguments.Options, arguments.AllowMultiple)
	if err != nil {
		return chatToolExecution{result: encodeChatToolResult(false, err.Error()), status: chatToolContinue}
	}
	if cancelled {
		return a.storeInteraction(signature, arguments.Options, nil)
	}
	return a.storeInteraction(signature, arguments.Options, selected)
}

func (a *chatAgent) storeInteraction(signature string, options []chatUserOption, selected []string) chatToolExecution {
	id := newChatID()
	confirmed := false
	for _, value := range selected {
		if value == "confirm" {
			confirmed = true
			break
		}
	}
	a.interactions[id] = chatUserInteraction{ID: id, Selected: selected, Confirmed: confirmed}
	for _, selectedID := range selected {
		if document, ok := a.documents[selectedID]; ok {
			document.Confirmed = true
		}
		for _, option := range options {
			if option.ID != selectedID {
				continue
			}
			if document, ok := a.documents[strings.TrimSpace(option.DocumentID)]; ok {
				document.Confirmed = true
			}
		}
	}
	if confirmed && a.pendingConfig != nil {
		a.pendingConfig.ConfirmationID = id
	}
	selectedOptions := make([]chatUserOption, 0, len(selected))
	for _, option := range options {
		for _, selectedID := range selected {
			if option.ID == selectedID {
				selectedOptions = append(selectedOptions, option)
				break
			}
		}
	}
	result := encodeChatJSON(map[string]interface{}{
		"ok":               true,
		"interaction_id":   id,
		"selected":         selected,
		"selected_options": selectedOptions,
		"cancelled":        len(selected) == 0,
		"options":          options,
	})
	a.lastInteractionSignature = signature
	a.lastInteractionResult = result
	return chatToolExecution{result: result, status: chatToolContinue}
}

func hasConfirmationOptions(options []chatUserOption) bool {
	seen := map[string]bool{}
	for _, option := range options {
		seen[option.ID] = true
	}
	return seen["confirm"] && seen["cancel"]
}

func selectUserOptions(value string, options []chatUserOption, allowMultiple bool) ([]string, bool, error) {
	if strings.EqualFold(value, "q") || strings.EqualFold(value, "quit") || strings.EqualFold(value, "cancel") || value == "取消" {
		return nil, true, nil
	}
	parts := strings.Split(value, ",")
	if !allowMultiple && len(parts) != 1 {
		return nil, false, errors.New("select one option number")
	}
	selected := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		index := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &index); err != nil || index < 1 || index > len(options) {
			return nil, false, errors.New("invalid option; enter a listed number or q to cancel")
		}
		id := options[index-1].ID
		if !seen[id] {
			selected = append(selected, id)
			seen[id] = true
		}
	}
	return selected, false, nil
}

func newChatID() string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	return fmt.Sprintf("chat-%d", os.Getpid())
}
