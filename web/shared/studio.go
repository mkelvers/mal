package shared

import "mal/integrations/jikan"

// GetProducerName extracts the default title from producer response
func GetProducerName(producer jikan.ProducerResponse) string {
	for _, title := range producer.Data.Titles {
		if title.Type == "Default" {
			return title.Title
		}
	}
	return "Studio"
}
