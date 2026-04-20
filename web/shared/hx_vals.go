package shared

import "encoding/json"

func HxVals(v map[string]interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
