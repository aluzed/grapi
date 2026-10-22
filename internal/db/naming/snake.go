package naming

import "github.com/aluzed/grapi/schema"

// SnakeCase is lodash's snakeCase.
func SnakeCase(s string) string { return schema.SnakeCase(s) }
