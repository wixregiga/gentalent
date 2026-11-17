package programs

import (
	"strconv"
	"strings"
	"testing"
)

func TestDecodeManifestUnwrapsEveryShapeTheCLIsPrint(t *testing.T) {
	quoted := strconv.Quote(manifestJSON)
	shapes := map[string]string{
		"plain manifest":           manifestJSON,
		"surrounding whitespace":   "\n  " + manifestJSON + "\n\n",
		"structured_output":        `{"type":"result","structured_output":` + manifestJSON + `}`,
		"result object":            `{"type":"result","result":` + manifestJSON + `}`,
		"result string":            `{"type":"result","result":` + quoted + `}`,
		"result string with fence": `{"result":` + strconv.Quote("Here you go:\n```json\n"+manifestJSON+"\n```") + `}`,
		"JSON string holding JSON": quoted,
		"fenced block":             "Done.\n```json\n" + manifestJSON + "\n```\nBye.",
		"fence without language":   "```\n" + manifestJSON + "\n```",
		"prose around the object":  "I found them. " + manifestJSON + " Hope that helps.",
	}
	for label, output := range shapes {
		manifest, err := decodeManifest(output)
		if err != nil {
			t.Errorf("%s: %v", label, err)
			continue
		}
		if manifest.CanonicalName != "Aster Example" || len(manifest.Photos) != 3 || manifest.Photos[2].Caption != "Shot 3" {
			t.Errorf("%s: %+v", label, manifest)
		}
	}
}

func TestDecodeManifestPrefersAManifestOverItsResultField(t *testing.T) {
	photos := manifestJSON[strings.Index(manifestJSON, `"photos":`):]
	output := `{"result":"not the manifest","canonical_name":"Aster Example","summary":"s",` + photos
	manifest, err := decodeManifest(output)
	if err != nil || manifest.Summary != "s" {
		t.Fatalf("%+v, %v", manifest, err)
	}
}

func TestDecodeManifestRefusesOutputThatIsNotAValidManifest(t *testing.T) {
	cases := map[string]string{
		"empty":                "  \n",
		"no JSON":              "I could not find any photos, sorry.",
		"a JSON array":         `["a"]`,
		"a bare number":        `7`,
		"broken fence":         "```json\n{not json}\n```",
		"two photos":           `{"canonical_name":"A","summary":"s","photos":[{"raw_filename":"1.jpg","source_page_url":"u","image_url":"i","caption":"c"},{"raw_filename":"2.jpg","source_page_url":"u","image_url":"i","caption":"c"}]}`,
		"missing canonical":    strings.Replace(manifestJSON, `"canonical_name":"Aster Example",`, "", 1),
		"filename escapes":     strings.Replace(manifestJSON, `"01.jpg"`, `"../01.jpg"`, 1),
		"result not an object": `{"result":["x"]}`,
		"result string prose":  `{"result":"all done"}`,
	}
	for label, output := range cases {
		if _, err := decodeManifest(output); err == nil {
			t.Errorf("%s accepted", label)
		}
	}
}

func TestSubscriptionEnvironmentDropsOnlyTheKeyVariables(t *testing.T) {
	base := []string{"PATH=/bin", "OPENAI_API_KEY=a", "ANTHROPIC_API_KEY=b", "CLAUDE_API_KEY=c", "GEMINI_API_KEY=d",
		"GOOGLE_API_KEY=e", "ELECTRON_RUN_AS_NODE=1", "OPENAI_API_KEY_FILE=keepme", "HOME=/h"}
	got := strings.Join(subscriptionEnvironment(base), " ")
	if got != "PATH=/bin OPENAI_API_KEY_FILE=keepme HOME=/h" {
		t.Fatalf("got %s", got)
	}
}
