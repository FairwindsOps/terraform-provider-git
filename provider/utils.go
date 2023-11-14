package provider

func getMapItem(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}

	list := value.([]interface{})
	if len(list) == 0 {
		return nil
	}

	data := list[0]
	return data.(map[string]interface{})
}
