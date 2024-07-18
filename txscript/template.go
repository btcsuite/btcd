package txscript

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"html/template"
	"strconv"
	"strings"
)

// ScriptTemplateOpt is a function type for configuring the script template.
type ScriptTemplateOption func(*templateConfig)

// templateConfig holds the configuration for the script template.
type templateConfig struct {
	params map[string]interface{}

	customFuncs template.FuncMap
}

// WithScriptTemplateParams adds parameters to the script template.
func WithScriptTemplateParams(params map[string]interface{}) ScriptTemplateOption {
	return func(cfg *templateConfig) {
		for k, v := range params {
			cfg.params[k] = v
		}
	}
}

// WithCustomTemplateFunc adds a custom function to the template.
func WithCustomTemplateFunc(name string, fn interface{}) ScriptTemplateOption {
	return func(cfg *templateConfig) {
		cfg.customFuncs[name] = fn
	}
}

// ScriptTemplate processes a script template with parameters and returns the
// corresponding script bytes. This functions allows Bitcoin scripts to be
// created using a DSL-like syntax, based on Go's templating system.
//
// An example of a simple p2pkh template would be:
//
//	`OP_DUP OP_HASH160 0x14e8948c7afa71b6e6fad621256474b5959e0305 OP_EQUALVERIFY OP_CHECKSIG`
//
// Strings that have the `0x` prefix are assumed to byte strings to be pushed
// ontop of the stack. Integers can be passed as normal. If a value can't be
// parsed as an integer, then it's assume that it's a byte slice without the 0x
// prefix.
//
// Normal go template operations can be used as well. The params argument
// houses paramters to pass into the script, for example a local variable
// storing a computed public key.
func ScriptTemplate(scriptTmpl string, opts ...ScriptTemplateOption) ([]byte, error) {
	cfg := &templateConfig{
		params:      make(map[string]interface{}),
		customFuncs: make(template.FuncMap),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	funcMap := template.FuncMap{
		"hex":        hexEncode,
		"hex_str":    hexStr,
		"unhex":      hexDecode,
		"range_iter": rangeIter,
	}

	for k, v := range cfg.customFuncs {
		funcMap[k] = v
	}

	tmpl, err := template.New("script").Funcs(funcMap).Parse(scriptTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg.params); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return processScript(buf.String())
}

// looksLikeInt checks if a string looks like an integer.
func looksLikeInt(s string) bool {
	// Check if the string starts with an optional sign.
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}

	// Check if the remaining string contains only digits.
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}

	return len(s) > 0
}


// processScript converts the template output to actual script bytes. We scan
// each line, then go through each element one by one, deciding to either add a
// normal op code, a push data, or an integer value.
func processScript(script string) ([]byte, error) {
	var builder ScriptBuilder

	// We'll a bufio scanner to take care of some of the parsing for us.
	// bufio.ScanWords will split on word boundaries, based on unicode
	// characters.
	scanner := bufio.NewScanner(strings.NewReader(script))
	scanner.Split(bufio.ScanWords)

	// Run through each word, deciding if we should add an op code, a push
	// data, or an integer value.
	for scanner.Scan() {
		token := scanner.Text()
		switch {
		// If it starts with OP_, then we'll try to parse out the op
		// code.
		case strings.HasPrefix(token, "OP_"):
			opcode, ok := OpcodeByName[token]
			if !ok {
				return nil, fmt.Errorf("unknown opcode: "+
					"%s", token)
			}

			builder.AddOp(opcode)

		// If it has an 0x prefix, then we'll try to decode it as a hex
		// string to push data.
		case strings.HasPrefix(token, "0x"):
			data, err := hex.DecodeString(
				strings.TrimPrefix(token, "0x"),
			)
			if err != nil {
				return nil, fmt.Errorf("invalid hex "+
					"data: %s", token)
			}

			builder.AddData(data)

		// Next, we'll try to parse ints for the integer op code.
		case looksLikeInt(token):
			val, err := strconv.ParseInt(token, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid "+
					"integer: %s", token)
			}

			builder.AddInt64(val)

		// Otherwise, we assume it's a byte string without the 0x
		// prefix.
		default:
			data, err := hex.DecodeString(token)
			if err != nil {
				return nil, fmt.Errorf("invalid token: %s",
					token)
			}

			builder.AddData(data)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading script: %w", err)
	}

	return builder.Script()
}

// rangeIter is useful for being able to execute a bounded for loop.
func rangeIter(start, end int) []int {
	var result []int

	for i := start; i < end; i++ {
		result = append(result, i)
	}

	return result
}

// hexEncode is a helper function to encode bytes to hex in templates.
// It adds the "0x" prefix to ensure the output is processed as hex data
// and not misinterpreted as an integer.
func hexEncode(data []byte) string {
	return "0x" + hex.EncodeToString(data)
}

// hexStr is a helper function to encode bytes to a raw hex string in templates
// without the "0x" prefix.
func hexStr(data []byte) string {
	return hex.EncodeToString(data)
}

// hexDecode is a helper function to decode hex to bytes in templates
func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}

// Example usage:
func ExampleScriptTemplate() {
	localPubkey, _ := hex.DecodeString("14e8948c7afa71b6e6fad621256474b5959e0305")

	scriptBytes, err := ScriptTemplate(`
		OP_DUP OP_HASH160 0x14e8948c7afa71b6e6fad621256474b5959e0305 OP_EQUALVERIFY OP_CHECKSIG
		OP_DUP OP_HASH160 {{ hex .LocalPubkeyHash }} OP_EQUALVERIFY OP_CHECKSIG
		{{ .Timeout }} OP_CHECKLOCKTIMEVERIFY OP_DROP
		
		{{- range $i := range_iter 0 3 }}
			{{ add 10 $i }} OP_ADD
		{{- end }}`,
		WithScriptTemplateParams(map[string]interface{}{
			"LocalPubkeyHash": localPubkey,
			"Timeout":         1,
		}),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	asmScript, err := DisasmString(scriptBytes)
	if err != nil {
		fmt.Printf("Error converting to ASM: %v\n", err)
		return
	}

	fmt.Printf("Script ASM:\n%s\n", asmScript)
}
