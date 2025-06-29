package tools

func EchoTool(params map[string]interface{}) (map[string]interface{}, error) {
    msg := params["input"].(string)
    return map[string]interface{}{"result": msg}, nil
}
