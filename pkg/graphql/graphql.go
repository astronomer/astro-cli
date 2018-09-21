package graphql

import (
	"fmt"
	"strings"
)

type Query struct {
	Alias       string
	Action      string
	Parameters  []Parameter
	ReturnAttrs []string
}

type Mutation struct {
	Alias       string
	Action      string
	Parameters  []Parameter
	ReturnAttrs []string
}

type Parameter struct {
	Name       string
	Value      string
	IsRequired bool
	Type       string
	IsQuoted   bool
}

func (q Query) GetQueryStr() (string, error) {
	// qtemp holds tmeplate for graphql query
	// query type | query alias | query action/resource | built out variable string | return attributes
	qTemp := "query %s { \n\t%s%s{\t%s\n\t}\n}"

	// pTemp holds template for graphql query variables
	pTemp := ""
	// aTemp holds the template for graphql return attributes
	aTemp := ""

	// Build parameter string
	for _, p := range q.Parameters {
		s, err := p.getVariableStr()
		if err != nil {
			return "", err
		}

		pTemp += s
	}

	// Remove last comma appended to parameter string
	pTemp = strings.TrimSuffix(pTemp, ",")

	// Wrap parameters in parens
	pTemp = "(" + pTemp + "\n\t)"

	// Build return attr string
	for _, a := range q.ReturnAttrs {
		aTemp += fmt.Sprintf("\n\t\t%s", a)
	}

	return fmt.Sprintf(qTemp, q.Alias, q.Action, pTemp, aTemp), nil
}

func (m Mutation) GetQueryStr() (string, error) {
	// qtemp holds tmeplate for graphql query
	// query type | query alias | query action/resource | built out variable string | return attributes
	mTemp := "mutation %s { \n\t%s%s{\t%s\n\t}\n}"

	// pTemp holds template for graphql query variables
	pTemp := ""
	// aTemp holds the template for graphql return attributes
	aTemp := ""

	// Build parameter string
	for _, p := range m.Parameters {
		s, err := p.getVariableStr()
		if err != nil {
			return "", err
		}

		pTemp += s
	}

	// Remove last comma appended to parameter string
	pTemp = strings.TrimSuffix(pTemp, ",")

	// Wrap parameters in parens
	pTemp = "(" + pTemp + "\n\t)"

	// Build return attr string
	for _, a := range m.ReturnAttrs {
		aTemp += fmt.Sprintf("\n\t\t%s", a)
	}

	return fmt.Sprintf(mTemp,
		m.Alias,
		m.Action,
		pTemp,
		aTemp,
	), nil
}

func (p Parameter) getVariableStr() (string, error) {
	temp := "\n\t\t%s:%s,"
	value := ""
	// Return error, required param not provided
	if p.IsRequired && len(p.Value) == 0 {
		return "", fmt.Errorf("no value specified for required parameter %s", p.Name)
	}

	// Return empty string since a non-required param was not provided
	if !p.IsRequired && len(p.Value) == 0 {
		return value, nil
	}

	// Wrap value in quotes if needed
	if p.IsQuoted {
		value = fmt.Sprintf("\"%s\"", p.Value)
	} else {
		value = p.Value
	}

	return fmt.Sprintf(temp, p.Name, value), nil
}
