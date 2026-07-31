package tools

import (
	"encoding/json"
	"net/http"

	"github.com/Francesco99975/shorehamex2/internal/enums"
)

func SetToastTrigger(w http.ResponseWriter, kind enums.MessageKind, message string) {
	payload := map[string]any{
		"toast": map[string]any{
			"kind": string(kind),
			"msg":  message,
		},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	w.Header().Set("HX-Trigger", string(b))
}
