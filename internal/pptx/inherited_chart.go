package pptx

import (
	"fmt"
	"github.com/beevik/etree"
	"github.com/xuri/excelize/v2"
	"strconv"
	"strings"
)

func inheritedColumnChart(c Chart) (*etree.Document, error) {
	var b strings.Builder
	b.WriteString(`<chartSpace xmlns="` + cns + `" xmlns:a="` + ans + `"><date1904 val="0"/><chart><autoTitleDeleted val="0"/><plotArea><barChart><barDir val="col"/><grouping val="clustered"/>`)
	cache := func(kind, formula string, values []string) {
		b.WriteString(`<` + kind + `Ref><f>` + esc(formula) + `</f><` + kind + `Cache>`)
		if kind == "num" {
			b.WriteString(`<formatCode>General</formatCode>`)
		}
		fmt.Fprintf(&b, `<ptCount val="%d"/>`, len(values))
		for i, v := range values {
			fmt.Fprintf(&b, `<pt idx="%d"><v>%s</v></pt>`, i, esc(v))
		}
		b.WriteString(`</` + kind + `Cache></` + kind + `Ref>`)
	}
	for i, s := range c.Series {
		col, _ := excelize.ColumnNumberToName(i + 2)
		fmt.Fprintf(&b, `<ser><idx val="%d"/><order val="%d"/><tx>`, i, i)
		cache("str", "Sheet1!$"+col+"$1", []string{s.Name})
		b.WriteString(`</tx><cat>`)
		cache("str", fmt.Sprintf("Sheet1!$A$2:$A$%d", len(c.Categories)+1), c.Categories)
		b.WriteString(`</cat><val>`)
		values := []string{}
		for _, v := range s.Values {
			values = append(values, strconv.FormatFloat(v, 'f', -1, 64))
		}
		cache("num", fmt.Sprintf("Sheet1!$%s$2:$%s$%d", col, col, len(values)+1), values)
		b.WriteString(`</val></ser>`)
	}
	b.WriteString(`<axId val="-2068027336"/><axId val="-2113994440"/></barChart><catAx><axId val="-2068027336"/><scaling><orientation val="minMax"/></scaling><delete val="0"/><axPos val="b"/><majorTickMark val="out"/><minorTickMark val="none"/><tickLblPos val="nextTo"/><crossAx val="-2113994440"/><crosses val="autoZero"/><auto val="1"/><lblAlgn val="ctr"/><lblOffset val="100"/><noMultiLvlLbl val="0"/></catAx><valAx><axId val="-2113994440"/><scaling/><delete val="0"/><axPos val="l"/><majorGridlines/><majorTickMark val="out"/><minorTickMark val="none"/><tickLblPos val="nextTo"/><crossAx val="-2068027336"/><crosses val="autoZero"/></valAx></plotArea><dispBlanksAs val="gap"/></chart><txPr><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1800"/></a:pPr><a:endParaRPr lang="en-US"/></a:p></txPr></chartSpace>`)
	d := etree.NewDocument()
	err := d.ReadFromString(b.String())
	return d, err
}
