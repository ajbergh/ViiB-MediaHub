package llm

// parseTOONLine is retained as a compatibility parser while enrichment format
// handling continues to evolve. Keep the reference explicit so Staticcheck can
// distinguish intentional retention from accidentally dead code.
var _ = parseTOONLine
