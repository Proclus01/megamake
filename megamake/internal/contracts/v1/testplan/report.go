package testplan

import (
	"sort"
	"strings"

	contractartifact "github.com/megamake/megamake/internal/contracts/v1/artifact"
)

type TestLevelV1 string

const (
	LevelSmoke      TestLevelV1 = "smoke"
	LevelUnit       TestLevelV1 = "unit"
	LevelIntegration TestLevelV1 = "integration"
	LevelE2E        TestLevelV1 = "e2e"
	LevelRegression TestLevelV1 = "regression"
)

type LevelSetV1 struct {
	Include map[TestLevelV1]bool `json:"include"`
}

func ParseLevelSetV1(csv string) LevelSetV1 {
	out := LevelSetV1{Include: map[TestLevelV1]bool{}}
	if strings.TrimSpace(csv) == "" {
		out.Include[LevelSmoke] = true
		out.Include[LevelUnit] = true
		out.Include[LevelIntegration] = true
		out.Include[LevelE2E] = true
		out.Include[LevelRegression] = true
		return out
	}
	items := strings.Split(csv, ",")
	for _, it := range items {
		x := strings.ToLower(strings.TrimSpace(it))
		switch TestLevelV1(x) {
		case LevelSmoke, LevelUnit, LevelIntegration, LevelE2E, LevelRegression:
			out.Include[TestLevelV1(x)] = true
		}
	}
	return out
}

func (s LevelSetV1) Has(l TestLevelV1) bool {
	return s.Include[l]
}

type SubjectKindV1 string

const (
	KindFunction   SubjectKindV1 = "function"
	KindMethod     SubjectKindV1 = "method"
	KindClass      SubjectKindV1 = "class"
	KindEndpoint   SubjectKindV1 = "endpoint"
	KindEntrypoint SubjectKindV1 = "entrypoint"
	KindModule     SubjectKindV1 = "module"
)

type SubjectParamV1 struct {
	Name     string `json:"name"`
	TypeHint string `json:"typeHint,omitempty"`
	Optional bool   `json:"optional"`
}

type IOCapabilitiesV1 struct {
	ReadsFS     bool `json:"readsFS"`
	WritesFS    bool `json:"writesFS"`
	Network     bool `json:"network"`
	DB          bool `json:"db"`
	Env         bool `json:"env"`
	Concurrency bool `json:"concurrency"`
}

type TestSubjectV1 struct {
	ID          string            `json:"id"`
	Kind        SubjectKindV1     `json:"kind"`
	Language    string            `json:"language"`
	Name        string            `json:"name"`
	Path        string            `json:"path"` // POSIX relpath
	Signature   string            `json:"signature,omitempty"`
	Exported    bool              `json:"exported"`
	Params      []SubjectParamV1  `json:"params,omitempty"`
	RiskScore   int               `json:"riskScore"`
	RiskFactors []string          `json:"riskFactors,omitempty"`
	IO          IOCapabilitiesV1  `json:"io"`
	Meta        map[string]string `json:"meta,omitempty"`
}

type ScenarioSuggestionV1 struct {
	Level      TestLevelV1 `json:"level"`
	Title      string      `json:"title"`
	Rationale  string      `json:"rationale"`
	Steps      []string    `json:"steps,omitempty"`
	Inputs     []string    `json:"inputs,omitempty"`
	Assertions []string    `json:"assertions,omitempty"`
}

type CoverageFlagV1 string

const (
	CoverageGreen  CoverageFlagV1 = "green"
	CoverageYellow CoverageFlagV1 = "yellow"
	CoverageRed    CoverageFlagV1 = "red"
)

type CoverageEvidenceV1 struct {
	File string `json:"file"`
	Hits int    `json:"hits"`
}

type CoverageV1 struct {
	Flag     CoverageFlagV1        `json:"flag"`
	Status   string               `json:"status"` // DONE/PARTIAL/MISSING
	Score    int                  `json:"score"`
	Evidence []CoverageEvidenceV1 `json:"evidence,omitempty"`
	Notes    []string             `json:"notes,omitempty"`
}

type SubjectPlanV1 struct {
	Subject   TestSubjectV1           `json:"subject"`
	Scenarios []ScenarioSuggestionV1  `json:"scenarios,omitempty"`
	Coverage  CoverageV1              `json:"coverage"`
}

type LanguagePlanV1 struct {
	Name          string          `json:"name"`
	Frameworks    []string        `json:"frameworks,omitempty"`
	Subjects      []SubjectPlanV1 `json:"subjects"`
	TestFilesFound int            `json:"testFilesFound"`
}

type PlanSummaryV1 struct {
	TotalLanguages int `json:"totalLanguages"`
	TotalSubjects  int `json:"totalSubjects"`
	TotalScenarios int `json:"totalScenarios"`
}

type TestPlanReportV1 struct {
	Languages   []LanguagePlanV1 `json:"languages"`
	GeneratedAt string          `json:"generatedAt"`
	Summary     PlanSummaryV1   `json:"summary"`
	Warnings    []string        `json:"warnings,omitempty"`
}

// ToXML renders a pseudo-XML test plan similar to your Swift output style.
func (r TestPlanReportV1) ToXML() string {
	var parts []string
	parts = append(parts, "<test_plan generatedAt=\""+contractartifact.EscapeAttr(r.GeneratedAt)+"\">")

	for _, lp := range r.Languages {
		fw := strings.Join(lp.Frameworks, ", ")
		parts = append(parts, "  <language name=\""+contractartifact.EscapeAttr(lp.Name)+"\" frameworks=\""+contractartifact.EscapeAttr(fw)+"\" testFilesFound=\""+itoa(lp.TestFilesFound)+"\">")
		for _, sp := range lp.Subjects {
			s := sp.Subject
			parts = append(parts, "    <subject id=\""+contractartifact.EscapeAttr(s.ID)+"\" kind=\""+contractartifact.EscapeAttr(string(s.Kind))+"\" language=\""+contractartifact.EscapeAttr(s.Language)+"\" name=\""+contractartifact.EscapeAttr(s.Name)+"\" path=\""+contractartifact.EscapeAttr(s.Path)+"\" exported=\""+boolAttr(s.Exported)+"\">")
			if strings.TrimSpace(s.Signature) != "" {
				parts = append(parts, "      <signature><![CDATA["+s.Signature+"]]></signature>")
			}

			parts = append(parts, "      <coverage flag=\""+contractartifact.EscapeAttr(string(sp.Coverage.Flag))+"\" status=\""+contractartifact.EscapeAttr(sp.Coverage.Status)+"\" score=\""+itoa(sp.Coverage.Score)+"\">")
			for _, n := range sp.Coverage.Notes {
				parts = append(parts, "        <note><![CDATA["+n+"]]></note>")
			}
			for _, ev := range sp.Coverage.Evidence {
				parts = append(parts, "        <evidence file=\""+contractartifact.EscapeAttr(ev.File)+"\" hits=\""+itoa(ev.Hits)+"\" />")
			}
			parts = append(parts, "      </coverage>")

			if len(s.Params) > 0 {
				parts = append(parts, "      <params>")
				for _, p := range s.Params {
					parts = append(parts, "        <param name=\""+contractartifact.EscapeAttr(p.Name)+"\" optional=\""+boolAttr(p.Optional)+"\" typeHint=\""+contractartifact.EscapeAttr(p.TypeHint)+"\"/>")
				}
				parts = append(parts, "      </params>")
			}
			if len(s.RiskFactors) > 0 {
				parts = append(parts, "      <risk score=\""+itoa(s.RiskScore)+"\">")
				for _, rf := range s.RiskFactors {
					parts = append(parts, "        <factor><![CDATA["+rf+"]]></factor>")
				}
				parts = append(parts, "      </risk>")
			} else {
				parts = append(parts, "      <risk score=\""+itoa(s.RiskScore)+"\"/>")
			}

			if len(s.Meta) > 0 {
				parts = append(parts, "      <meta>")
				keys := make([]string, 0, len(s.Meta))
				for k := range s.Meta {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					parts = append(parts, "        <item key=\""+contractartifact.EscapeAttr(k)+"\" value=\""+contractartifact.EscapeAttr(s.Meta[k])+"\"/>")
				}
				parts = append(parts, "      </meta>")
			}

			for _, sc := range sp.Scenarios {
				parts = append(parts, "      <scenario level=\""+contractartifact.EscapeAttr(string(sc.Level))+"\">")
				parts = append(parts, "        <title><![CDATA["+sc.Title+"]]></title>")
				parts = append(parts, "        <rationale><![CDATA["+sc.Rationale+"]]></rationale>")
				if len(sc.Inputs) > 0 {
					parts = append(parts, "        <inputs>")
					for _, in := range sc.Inputs {
						parts = append(parts, "          <case><![CDATA["+in+"]]></case>")
					}
					parts = append(parts, "        </inputs>")
				}
				if len(sc.Steps) > 0 {
					parts = append(parts, "        <steps>")
					for _, st := range sc.Steps {
						parts = append(parts, "          <step><![CDATA["+st+"]]></step>")
					}
					parts = append(parts, "        </steps>")
				}
				if len(sc.Assertions) > 0 {
					parts = append(parts, "        <assertions>")
					for _, a := range sc.Assertions {
						parts = append(parts, "          <assert><![CDATA["+a+"]]></assert>")
					}
					parts = append(parts, "        </assertions>")
				}
				parts = append(parts, "      </scenario>")
			}

			parts = append(parts, "    </subject>")
		}
		parts = append(parts, "  </language>")
	}

	parts = append(parts, "  <summary languages=\""+itoa(r.Summary.TotalLanguages)+"\" subjects=\""+itoa(r.Summary.TotalSubjects)+"\" scenarios=\""+itoa(r.Summary.TotalScenarios)+"\"/>")

	if len(r.Warnings) > 0 {
		parts = append(parts, "  <warnings>")
		for _, w := range r.Warnings {
			parts = append(parts, "    <warning><![CDATA["+w+"]]></warning>")
		}
		parts = append(parts, "  </warnings>")
	}

	parts = append(parts, "</test_plan>")
	return strings.Join(parts, "\n")
}

func boolAttr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	return sign + string(buf[i:])
}
