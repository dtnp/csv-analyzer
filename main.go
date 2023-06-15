package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type TypeTotals struct {
	Nil    int
	Bool   int
	Int    int
	Float  int
	String int
	JSON   int
	Array  int
}

type TypeAssert struct {
	Value          interface{}
	ConvertedValue interface{}
	ValueType      string
	IsArray        bool
	IsJson         bool
	IsTruthy       bool
	TruthyValue    bool
}

type FileDetails struct {
	TotalLineCount    int
	FirstRowAreLabels bool
	ColumnsCounts     map[string]int
	ColumnTypeCounts  TypeTotals
	ColumnTypeDetails []TypeAssert
	ParseStartTime    time.Time
	ParseEndTime      time.Time
	ParseTime         time.Duration
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("ERROR: %s", err)
		os.Exit(1)
	}
}

func run() error {
	fd := FileDetails{
		ParseStartTime: time.Now(),
	}

	// Open the file
	csvfile, err := os.Open("annual-enterprise-survey-2021-financial-year-provisional-csv.csv")
	if err != nil {
		return err
	}

	// Parse the file
	r := csv.NewReader(csvfile)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	var first5Rows [][]string
	columnCounts := make(map[string]int)
	var normalRow []string

	// Iterate through the records
	for {
		// Read each record from csv
		row, err := r.Read()

		colLen := fmt.Sprintf("%d", len(row))
		_, ok := columnCounts[colLen]
		if colLen != "0" && !ok {
			columnCounts[colLen] = 0
		}

		if err == io.EOF {
			break
		}
		if colLen == "0" {
			continue
		}
		columnCounts[colLen] += 1

		if err != nil {
			if strings.Contains(err.Error(), "wrong number of fields") {
				fd.TotalLineCount += 1
				continue
			}

			fmt.Printf("\n\n%s\n\n", err.Error())
			pp, _ := json.MarshalIndent(row, "", " ")
			fmt.Printf("Broken Row:\n%s\n", string(pp))

			return err
		}

		if fd.TotalLineCount < 5 {
			first5Rows = append(first5Rows, row)
		}

		if fd.TotalLineCount < 3 {
			normalRow = row
		}

		fd.TotalLineCount += 1
	}

	fd.ColumnsCounts = columnCounts
	fd.ParseEndTime = time.Now()
	fd.ParseTime = fd.ParseEndTime.Sub(fd.ParseStartTime)
	fd.FirstRowAreLabels = IsFirstRowLabels(first5Rows, 5)

	tt, tas := GetTypeTotals(normalRow)
	fd.ColumnTypeCounts = tt
	fd.ColumnTypeDetails = tas

	pp, _ := json.MarshalIndent(fd, "", " ")
	fmt.Printf("File Details:\n%s\n", string(pp))
	fmt.Printf("Time in Seconds: %v\n", fd.ParseTime.Seconds())

	return nil
}

func GetTypeTotals(row []string) (TypeTotals, []TypeAssert) {
	var tt TypeTotals
	var ctd []TypeAssert
	for _, col := range row {
		ta, _ := TypeAsserter(col)
		ctd = append(ctd, ta)

		switch ta.ValueType {
		case "nil":
			tt.Nil += 1
		case "bool":
			tt.Bool += 1
		case "int":
			tt.Int += 1
		case "float":
			tt.Float += 1
		case "string":
			tt.String += 1
		case "array":
			tt.Array += 1
		case "json":
			tt.JSON += 1
		default:
		}
	}

	return tt, ctd
}

func IsFirstRowLabels(rows [][]string, rowsToCheck int) bool {
	// Make sure there are enough rows to check
	l := len(rows)
	if rowsToCheck > l {
		rowsToCheck = l
	}

	// Get first row initially to compare to the next few
	firstRowTypes := make([]TypeAssert, len(rows[0])+1)
	for i, col := range rows[0] {
		ta, err := TypeAsserter(col)
		if err != nil {
			fmt.Printf("IsFirstRowLabels: TypeAsserter: %s", err)
			continue
		}
		firstRowTypes[i] = ta
	}

	for i := 1; i < rowsToCheck; i++ {
		row := rows[i]
		for j, col := range row {
			if ta, err := TypeAsserter(col); err == nil {
				if ta.ValueType != firstRowTypes[j].ValueType {
					return true
				}
			}
		}
	}

	return false
}


func TypeAsserter(value interface{}) (TypeAssert, error) {

	var ta = TypeAssert{
		Value: value,
		IsArray: false,
		IsJson: false,
		IsTruthy: false,
		TruthyValue: false,
	}

	// everything might come back string - gross.  Lets force convert some stuff?
	changed := false
	v, err := strconv.ParseInt(fmt.Sprintf("%v", value),10,64)
	if err == nil {
		value = v
		changed = true
	}
	if !changed {
		v, err := strconv.ParseFloat(fmt.Sprintf("%s", value),64)
		if err == nil {
			value = v
			changed = true
		}
	}

	switch v := value.(type) {
	case nil:
		ta.ValueType = "nil"
		ta.ConvertedValue = nil
		ta.IsTruthy = true
		ta.TruthyValue = false
	case bool:
		ta.ValueType = "bool"
		ta.ConvertedValue = fmt.Sprintf("%t", value)
		ta.IsTruthy = true
		ta.TruthyValue = true
		if ta.ConvertedValue == "false" {
			ta.TruthyValue = false
		}
	case int, int8, int16, int32, int64:
		ta.ValueType = "int"
	case float32, float64:
		ta.ValueType = "float"
	case string, []byte:
		ta.ValueType = "string"
		ta.ConvertedValue = fmt.Sprintf("%s", value)
	case json.RawMessage:
		ta.ValueType = "json"
		ta.ConvertedValue = value
	default:
		ta.ValueType = "string"
		ta.ConvertedValue = value
		fmt.Printf("unknown type (%s): %T", value, v)
	}

	// Checking truthiness
	if ta.ValueType == "string" {
		v := fmt.Sprintf("%s", value)
		ta.IsTruthy, ta.TruthyValue = IsTruthyString(v)
		ta.IsArray = IsArrayString(v)
		ta.IsJson = IsJsonString(v)

	} else if ta.ValueType == "int" {
		v, err := strconv.Atoi(fmt.Sprintf("%d", value))
		if err != nil {
			return TypeAssert{}, err

		}
		ta.IsTruthy, ta.TruthyValue = IsTruthyInt(v)
		ta.ConvertedValue = v

	} else if ta.ValueType == "float" {
		v, err := strconv.ParseFloat(fmt.Sprintf("%f", value), 64)
		if err != nil {
			return TypeAssert{}, err
		}
		ta.IsTruthy, ta.TruthyValue = IsTruthyFloat(v)
		ta.ConvertedValue = v
	}

	return ta, nil
}

func IsTruthyFloat(value float64) (bool, bool) {
	// Is falsy, but value is false
	if value == 0.0 {
		return true, false
	}

	// Is falsy, but value is true
	if value == 1.0 {
		return true, true
	}

	return false, false
}

func IsTruthyInt(value int) (bool, bool) {
	// Is falsy, but value is false
	if value == 0 {
		return true, false
	}

	// Is falsy, but value is true
	if value == 1 {
		return true, true
	}

	return false, false
}

func IsTruthyString(value string) (bool, bool) {
	isTruthy := false
	truthy := false

	tmp := strings.ToLower(fmt.Sprintf("%s", value))
	if len(tmp) >= 10 {
		return false, false
	}

	switch tmp {
	case "true", "t":
		isTruthy = true
		truthy = true
	case "false", "f", "":
		isTruthy = true
		truthy = false
	case "0", "-0":
		isTruthy = true
		truthy = false
	case "1":
		isTruthy = true
		truthy = true
	case "null", "nil", "none":
		isTruthy = true
		truthy = false
	case "undefined", "nan":
		isTruthy = true
		truthy = false
	default:
		isTruthy = false
		truthy = false
	}

	return isTruthy, truthy
}

func IsJsonString(value string) bool {
	v := strings.TrimSpace(value)
	if string(v[0]) == "{" && string(v[len(v) -1]) == "}" {
		return true
	}

	return false
}

func IsArrayString(value string) bool {
	v := strings.TrimSpace(value)
	if string(v[0]) == "[" && string(v[len(v) -1]) == "]" {
		return true
	}

	return false
}