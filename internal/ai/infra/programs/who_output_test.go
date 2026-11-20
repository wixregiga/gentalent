package programs

import (
	"strconv"
	"strings"
	"testing"
)

const whoJSON = `{"found":true,"name":"Ada Lovelace","blurb":"English mathematician who wrote the first published program.",` +
	`"source_url":"https://en.wikipedia.org/wiki/Ada_Lovelace","photo_file":"photo.jpg"}`

const noMatchJSON = `{"found":false,"name":"","blurb":"","source_url":"","photo_file":""}`

func TestDecodeWhoAnswerUnwrapsEveryShapeTheCLIsPrint(t *testing.T) {
	quoted := strconv.Quote(whoJSON)
	shapes := map[string]string{
		"codex output file":        whoJSON + "\n",
		"claude envelope":          `{"type":"result","subtype":"success","is_error":false,"duration_ms":21450,"num_turns":4,"result":"","session_id":"00000000-0000-0000-0000-000000000000","total_cost_usd":0,"structured_output":` + whoJSON + `}`,
		"result object":            `{"type":"result","result":` + whoJSON + `}`,
		"result string":            `{"type":"result","result":` + quoted + `}`,
		"result string with fence": `{"result":` + strconv.Quote("Here you go:\n```json\n"+whoJSON+"\n```") + `}`,
		"JSON string holding JSON": quoted,
		"fenced block":             "Done.\n```json\n" + whoJSON + "\n```\nBye.",
		"prose around the object":  "I know who that is. " + whoJSON + " Hope that helps.",
	}
	for label, output := range shapes {
		answer, err := decodeWhoAnswer(output)
		if err != nil {
			t.Errorf("%s: %v", label, err)
			continue
		}
		if !answer.Found || answer.Name != "Ada Lovelace" || answer.PhotoFile != "photo.jpg" || answer.Source != "https://en.wikipedia.org/wiki/Ada_Lovelace" {
			t.Errorf("%s: %+v", label, answer)
		}
	}
}

func TestDecodeWhoAnswerReadsANoMatch(t *testing.T) {
	for label, output := range map[string]string{
		"plain":           noMatchJSON,
		"claude envelope": `{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":` + noMatchJSON + `}`,
		"guess kept":      `{"found":false,"name":"Someone","blurb":"Maybe.","source_url":"","photo_file":"../x.jpg"}`,
	} {
		answer, err := decodeWhoAnswer(output)
		if err != nil || answer.Found || answer.Name != "" {
			t.Errorf("%s: %+v, %v", label, answer, err)
		}
	}
}

func TestDecodeWhoAnswerRefusesOutputThatIsNotAValidAnswer(t *testing.T) {
	cases := map[string]string{
		"empty":                 "  \n",
		"no JSON":               "That sounds like Ada Lovelace.",
		"a JSON array":          `["Ada Lovelace"]`,
		"no found flag":         `{"name":"Ada Lovelace","blurb":"b","source_url":"","photo_file":""}`,
		"found as text":         strings.Replace(whoJSON, `"found":true`, `"found":"true"`, 1),
		"found without a name":  strings.Replace(whoJSON, `"name":"Ada Lovelace"`, `"name":"  "`, 1),
		"found without a blurb": strings.Replace(whoJSON, `"blurb":"English mathematician who wrote the first published program."`, `"blurb":""`, 1),
		"name not text":         strings.Replace(whoJSON, `"name":"Ada Lovelace"`, `"name":7`, 1),
		"photo escapes":         strings.Replace(whoJSON, `"photo.jpg"`, `"../photo.jpg"`, 1),
		"photo absolute":        strings.Replace(whoJSON, `"photo.jpg"`, `"/etc/passwd"`, 1),
		"claude error envelope": `{"type":"result","subtype":"error_max_turns","is_error":true,"result":""}`,
		"result string prose":   `{"result":"all done"}`,
		"photo manifest":        manifestJSON,
	}
	for label, output := range cases {
		if _, err := decodeWhoAnswer(output); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
}
