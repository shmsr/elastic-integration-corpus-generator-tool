// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License
// 2.0; you may not use this file except in compliance with the Elastic License
// 2.0.

// Package esql provides a parser for ES|QL queries used in Elastic alerting rules.
// This parser extracts the structural elements needed to generate test data
// that will trigger (or not trigger) alerts.
package esql

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// ES|QL lexer definition
var esqlLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `//[^\n]*`},
	{Name: "MultiComment", Pattern: `/\*[\s\S]*?\*/`},
	{Name: "whitespace", Pattern: `\s+`}, // lowercase = elided
	{Name: "Pipe", Pattern: `\|`},
	{Name: "Comma", Pattern: `,`},
	{Name: "Colon", Pattern: `:`},
	{Name: "Semicolon", Pattern: `;`},
	{Name: "LParen", Pattern: `\(`},
	{Name: "RParen", Pattern: `\)`},
	{Name: "LBracket", Pattern: `\[`},
	{Name: "RBracket", Pattern: `\]`},
	{Name: "Asterisk", Pattern: `\*`},
	{Name: "Plus", Pattern: `\+`},
	{Name: "Minus", Pattern: `-`},
	{Name: "Percent", Pattern: `%`},
	{Name: "GTE", Pattern: `>=`},
	{Name: "LTE", Pattern: `<=`},
	{Name: "NEQ", Pattern: `!=`},
	{Name: "EQ", Pattern: `==`},
	{Name: "GT", Pattern: `>`},
	{Name: "LT", Pattern: `<`},
	{Name: "Assign", Pattern: `=`},
	{Name: "Slash", Pattern: `/`},
	{Name: "Dot", Pattern: `\.`},
	{Name: "Number", Pattern: `[0-9]+(\.[0-9]+)?`},
	{Name: "String", Pattern: `"([^"\\]|\\.)*"`},
	{Name: "Backtick", Pattern: "`[^`]+`"},
	// Keywords must come before generic Ident
	{Name: "AND", Pattern: `(?i)AND`},
	{Name: "OR", Pattern: `(?i)OR`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_\.\-\*]*`},
})

// Query represents a parsed ES|QL query
type Query struct {
	Commands []*Command `parser:"@@+"`
}

// Command represents a single ES|QL command (FROM, STATS, EVAL, WHERE, etc.)
type Command struct {
	From  *FromCommand  `parser:"( @@"`
	Stats *StatsCommand `parser:"| Pipe? @@"`
	Eval  *EvalCommand  `parser:"| Pipe? @@"`
	Where *WhereCommand `parser:"| Pipe? @@"`
	Keep  *KeepCommand  `parser:"| Pipe? @@"`
	Sort  *SortCommand  `parser:"| Pipe? @@"`
	Limit *LimitCommand `parser:"| Pipe? @@ )"`
}

// FromCommand: FROM index-pattern
type FromCommand struct {
	Index string `parser:"'FROM' @Ident"`
}

// StatsCommand: STATS aggregations BY grouping
type StatsCommand struct {
	Aggregations []*Aggregation `parser:"'STATS' (@@ (Comma @@)*)?"`
	GroupBy      []*Field       `parser:"('BY' @@ (Comma @@)*)?"`
}

// Aggregation represents an aggregation like: alias=AVG(field) or alias=COUNT(*)
type Aggregation struct {
	Alias    string       `parser:"(@Ident Assign)?"`
	Function string       `parser:"@Ident LParen"`
	Star     string       `parser:"( @Asterisk"`
	Args     []*Expr      `parser:"| @@ (Comma @@)* )? RParen"`
	Where    *WhereClause `parser:"@@?"`
}

// WhereClause for aggregation-level filtering
type WhereClause struct {
	Condition *BoolExpr `parser:"'WHERE' @@"`
}

// EvalCommand: EVAL field = expression
type EvalCommand struct {
	Assignments []*Assignment `parser:"'EVAL' @@ (Comma @@)*"`
}

// Assignment represents: name = expression
type Assignment struct {
	Name string `parser:"@Ident Assign"`
	Expr *Expr  `parser:"@@"`
}

// WhereCommand: WHERE condition
type WhereCommand struct {
	Condition *BoolExpr `parser:"'WHERE' @@"`
}

// KeepCommand: KEEP field1, field2, ...
type KeepCommand struct {
	Fields []string `parser:"'KEEP' @(Backtick|Ident) (Comma @(Backtick|Ident))*"`
}

// SortCommand: SORT field ASC/DESC
type SortCommand struct {
	Fields []*SortField `parser:"'SORT' @@ (Comma @@)*"`
}

// SortField represents a field with optional ordering
type SortField struct {
	Field string `parser:"@(Backtick|Ident)"`
	Order string `parser:"@('ASC'|'DESC')?"`
}

// LimitCommand: LIMIT n
type LimitCommand struct {
	Count int `parser:"'LIMIT' @Number"`
}

// Field represents a field reference or assignment
type Field struct {
	Name string `parser:"@(Backtick|Ident)"`
	Expr *Expr  `parser:"(Assign @@)?"`
}

// BoolExpr represents a boolean expression (conditions)
type BoolExpr struct {
	Left  *Comparison `parser:"@@"`
	Op    string      `parser:"(@(AND|OR)"`
	Right *BoolExpr   `parser:"@@)?"`
}

// Comparison represents a comparison: expr op expr
type Comparison struct {
	Left  *Expr  `parser:"@@"`
	Op    string `parser:"@(GTE|LTE|NEQ|EQ|GT|LT)?"`
	Right *Expr  `parser:"@@?"`
}

// Expr represents an arithmetic expression
type Expr struct {
	Left  *Term   `parser:"@@"`
	Op    string  `parser:"@(Plus|Minus|Asterisk|Slash|Percent)?"`
	Right *Expr   `parser:"@@?"`
}

// Term represents a term in an expression
type Term struct {
	Number   *float64   `parser:"( @Number"`
	String   *string    `parser:"| @String"`
	Function *FuncCall  `parser:"| @@"`
	Field    *FieldRef  `parser:"| @@"`
	Paren    *ParenExpr `parser:"| @@ )"`
}

// FieldRef represents a field reference (possibly backtick-quoted)
type FieldRef struct {
	Name string `parser:"@(Backtick|Ident)"`
}

// FuncCall represents a function call: FUNC(args)
type FuncCall struct {
	Name string  `parser:"@Ident"`
	Star string  `parser:"LParen ( @Asterisk"`
	Args []*Expr `parser:"| @@ (Comma @@)* )? RParen"`
}

// ParenExpr represents a parenthesized expression
type ParenExpr struct {
	Expr *Expr `parser:"LParen @@ RParen"`
}

// Parser is the ES|QL parser
var Parser = participle.MustBuild[Query](
	participle.Lexer(esqlLexer),
	participle.Unquote("String"),
	participle.CaseInsensitive("Ident"),
	participle.UseLookahead(5),
)

// Parse parses an ES|QL query string
func Parse(query string) (*Query, error) {
	// Remove comments first for cleaner parsing
	query = removeComments(query)

	parsed, err := Parser.ParseString("", query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ES|QL: %w", err)
	}
	return parsed, nil
}

// removeComments strips // and /* */ comments from the query
func removeComments(query string) string {
	var result strings.Builder
	lines := strings.Split(query, "\n")
	for _, line := range lines {
		// Remove single-line comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			result.WriteString(line)
			result.WriteString(" ")
		}
	}
	return strings.TrimSpace(result.String())
}

// AlertConfig holds extracted information for generating alert data
type AlertConfig struct {
	Index        string
	DataStream   string
	Fields       []AlertField
	Conditions   []AlertCondition
	Evals        []EvalDef
	GroupByField string
}

// AlertField represents a field used in the alert
type AlertField struct {
	Name         string
	Alias        string
	Function     string // AVG, MAX, MIN, SUM, COUNT
	TriggerValue interface{}
	SafeValue    interface{}
}

// AlertCondition represents a condition in WHERE clause
type AlertCondition struct {
	Field     string
	Operator  string
	Threshold float64
}

// EvalDef represents an EVAL definition
type EvalDef struct {
	Name        string
	Expression  string
	IsRatio     bool
	Numerator   string
	Denominator string
}

// ExtractAlertConfig extracts alert configuration from a parsed query
func ExtractAlertConfig(q *Query) *AlertConfig {
	config := &AlertConfig{}

	aliasToField := make(map[string]string)

	for _, cmd := range q.Commands {
		if cmd.From != nil {
			config.Index = cmd.From.Index
			// Extract data stream from index pattern
			config.DataStream = extractDataStream(cmd.From.Index)
		}

		if cmd.Stats != nil {
			for _, agg := range cmd.Stats.Aggregations {
				field := AlertField{
					Alias:    agg.Alias,
					Function: strings.ToUpper(agg.Function),
				}

				// Extract field name from arguments
				if len(agg.Args) > 0 && agg.Args[0] != nil {
					field.Name = extractFieldName(agg.Args[0])
				}

				if agg.Alias != "" {
					aliasToField[agg.Alias] = field.Name
				}

				config.Fields = append(config.Fields, field)
			}

			// Extract GROUP BY field
			if len(cmd.Stats.GroupBy) > 0 {
				config.GroupByField = cmd.Stats.GroupBy[0].Name
			}
		}

		if cmd.Eval != nil {
			for _, assign := range cmd.Eval.Assignments {
				evalDef := EvalDef{
					Name:       assign.Name,
					Expression: exprToString(assign.Expr),
				}

				// Check if this is a ratio calculation
				if isRatioExpr(assign.Expr) {
					evalDef.IsRatio = true
					evalDef.Numerator, evalDef.Denominator = extractRatioParts(assign.Expr, aliasToField)
				}

				config.Evals = append(config.Evals, evalDef)
			}
		}

		if cmd.Where != nil {
			conditions := extractConditions(cmd.Where.Condition)
			config.Conditions = append(config.Conditions, conditions...)
		}
	}

	// Set trigger/safe values based on conditions and evals
	config.setTriggerValues(aliasToField)

	return config
}

// extractDataStream extracts the data stream name from an index pattern
func extractDataStream(index string) string {
	// e.g., metrics-mongodb.status-* -> mongodb.status
	parts := strings.Split(index, "-")
	if len(parts) >= 2 {
		return strings.TrimSuffix(parts[1], "*")
	}
	return index
}

// extractFieldName extracts the field name from an expression
func extractFieldName(expr *Expr) string {
	if expr == nil || expr.Left == nil {
		return ""
	}
	term := expr.Left
	if term.Field != nil {
		return cleanFieldName(term.Field.Name)
	}
	if term.Function != nil && len(term.Function.Args) > 0 {
		return extractFieldName(term.Function.Args[0])
	}
	if term.Paren != nil {
		return extractFieldName(term.Paren.Expr)
	}
	return ""
}

// cleanFieldName removes backticks from field names
func cleanFieldName(name string) string {
	return strings.Trim(name, "`")
}

// exprToString converts an expression back to string representation
func exprToString(expr *Expr) string {
	if expr == nil {
		return ""
	}
	result := termToString(expr.Left)
	if expr.Op != "" {
		result += " " + expr.Op + " " + exprToString(expr.Right)
	}
	return result
}

func termToString(term *Term) string {
	if term == nil {
		return ""
	}
	if term.Number != nil {
		return fmt.Sprintf("%v", *term.Number)
	}
	if term.String != nil {
		return *term.String
	}
	if term.Field != nil {
		return term.Field.Name
	}
	if term.Function != nil {
		return term.Function.Name + "(...)"
	}
	if term.Paren != nil {
		return "(" + exprToString(term.Paren.Expr) + ")"
	}
	return ""
}

// isRatioExpr checks if expression contains a division that's multiplied by 100 (percentage)
func isRatioExpr(expr *Expr) bool {
	exprStr := exprToString(expr)
	// Check for division pattern
	hasDiv := strings.Contains(exprStr, "/")
	// Check for percentage multiplication (* 100)
	hasMult := strings.Contains(exprStr, "* 100") || strings.Contains(exprStr, "*100")
	return hasDiv && hasMult
}

// extractRatioParts extracts numerator and denominator from a ratio expression
func extractRatioParts(expr *Expr, aliasToField map[string]string) (string, string) {
	// Look for pattern: (a / b) * 100
	if expr.Left != nil && expr.Left.Paren != nil {
		inner := expr.Left.Paren.Expr
		return extractDivisionParts(inner, aliasToField)
	}
	
	// Try from string representation
	exprStr := exprToString(expr)
	if strings.Contains(exprStr, "/") {
		parts := strings.Split(exprStr, "/")
		if len(parts) >= 2 {
			num := strings.TrimSpace(strings.Trim(parts[0], "("))
			denom := strings.TrimSpace(strings.Split(parts[1], ")")[0])
			denom = strings.TrimSpace(strings.Split(denom, "*")[0])
			
			// Map aliases to field names
			if field, ok := aliasToField[num]; ok {
				num = field
			}
			if field, ok := aliasToField[denom]; ok {
				denom = field
			}
			
			return num, denom
		}
	}
	
	return "", ""
}

// extractDivisionParts extracts left and right of a division
func extractDivisionParts(expr *Expr, aliasToField map[string]string) (string, string) {
	if expr == nil {
		return "", ""
	}
	
	// Look for division operator
	if expr.Op == "/" {
		num := extractExprIdentifier(expr.Left, aliasToField)
		denom := extractExprIdentifier(expr.Right.Left, aliasToField)
		return num, denom
	}
	
	// Recurse
	if expr.Right != nil {
		return extractDivisionParts(expr.Right, aliasToField)
	}
	
	return "", ""
}

// extractExprIdentifier extracts identifier from term
func extractExprIdentifier(term *Term, aliasToField map[string]string) string {
	if term == nil {
		return ""
	}
	var name string
	if term.Field != nil {
		name = cleanFieldName(term.Field.Name)
	} else if term.Paren != nil {
		name = exprToString(term.Paren.Expr)
	}
	
	// Map alias to field
	if field, ok := aliasToField[name]; ok {
		return field
	}
	return name
}

// extractConditions extracts conditions from a boolean expression
func extractConditions(expr *BoolExpr) []AlertCondition {
	var conditions []AlertCondition
	
	if expr == nil || expr.Left == nil {
		return conditions
	}
	
	// Extract from left comparison
	if expr.Left.Op != "" && expr.Left.Right != nil {
		cond := AlertCondition{
			Field:    extractComparisonField(expr.Left.Left),
			Operator: expr.Left.Op,
		}
		
		// Extract threshold value
		if expr.Left.Right != nil && expr.Left.Right.Left != nil {
			term := expr.Left.Right.Left
			if term.Number != nil {
				cond.Threshold = *term.Number
			}
		}
		
		conditions = append(conditions, cond)
	}
	
	// Recurse into right side if AND/OR
	if expr.Right != nil {
		conditions = append(conditions, extractConditions(expr.Right)...)
	}
	
	return conditions
}

func extractComparisonField(expr *Expr) string {
	if expr == nil || expr.Left == nil {
		return ""
	}
	term := expr.Left
	if term.Field != nil {
		return cleanFieldName(term.Field.Name)
	}
	return ""
}

// setTriggerValues calculates appropriate trigger and safe values
func (c *AlertConfig) setTriggerValues(aliasToField map[string]string) {
	// Build condition map by alias/field
	conditionByField := make(map[string]AlertCondition)
	for _, cond := range c.Conditions {
		conditionByField[cond.Field] = cond
	}

	// Check for ratio-based alerts
	var ratioEval *EvalDef
	var percentThreshold float64 = 80.0

	for i := range c.Evals {
		if c.Evals[i].IsRatio {
			ratioEval = &c.Evals[i]
			// Find the threshold for this eval's result
			if cond, ok := conditionByField[c.Evals[i].Name]; ok {
				percentThreshold = cond.Threshold
			}
			break
		}
	}

	// Set values for each field
	for i := range c.Fields {
		field := &c.Fields[i]
		alias := field.Alias
		if alias == "" {
			alias = field.Name
		}

		// Check if this field has a direct condition
		if cond, ok := conditionByField[alias]; ok && cond.Threshold > 0.001 {
			setValuesByCondition(field, cond)
			continue
		}

		// Check if this is part of a ratio
		if ratioEval != nil {
			if field.Name == ratioEval.Numerator || aliasToField[ratioEval.Numerator] == field.Name {
				// Numerator field - high for trigger
				field.TriggerValue = (percentThreshold + 10) * 1000.0
				field.SafeValue = (percentThreshold - 40) * 1000.0
				continue
			}
			if field.Name == ratioEval.Denominator || aliasToField[ratioEval.Denominator] == field.Name {
				// Denominator field - base value
				field.TriggerValue = 100000.0
				field.SafeValue = 100000.0
				continue
			}
		}

		// Default values
		field.TriggerValue = 100.0
		field.SafeValue = 10.0
	}
}

func setValuesByCondition(field *AlertField, cond AlertCondition) {
	switch cond.Operator {
	case ">":
		field.TriggerValue = cond.Threshold + (cond.Threshold * 0.2)
		field.SafeValue = cond.Threshold * 0.5
	case ">=":
		field.TriggerValue = cond.Threshold + (cond.Threshold * 0.1)
		field.SafeValue = cond.Threshold * 0.5
	case "<":
		field.TriggerValue = cond.Threshold * 0.5
		field.SafeValue = cond.Threshold * 2
	case "<=":
		field.TriggerValue = cond.Threshold * 0.9
		field.SafeValue = cond.Threshold * 2
	default:
		field.TriggerValue = cond.Threshold
		field.SafeValue = cond.Threshold * 0.5
	}
}
