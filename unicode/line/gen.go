// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

package main

import (
	"fmt"
	"log"
	"strings"
	"unicode"

	"golang.org/x/text/internal/gen"
	"golang.org/x/text/internal/segmenter"
	"golang.org/x/text/internal/triegen"
	"golang.org/x/text/internal/ucd"
)

// lbMap maps Line_Break UCD property value strings to property indices.
// AI and CJ are stored as distinct properties so CSS line-break modes can
// remap them at runtime.
var lbMap = map[string]uint8{
	"XX":  XX,
	"BK":  BK,
	"CR":  CR,
	"LF":  LF,
	"NL":  NL,
	"SP":  SP,
	"ZW":  ZW,
	"WJ":  WJ,
	"GL":  GL,
	"CL":  CL,
	"EX":  EX,
	"IS":  IS,
	"SY":  SY,
	"OP":  OP,
	"QU":  QU,
	"NS":  NS,
	"HY":  HY,
	"BA":  BA,
	"BB":  BB,
	"B2":  B2,
	"IN":  IN,
	"AL":  AL,
	"NU":  NU,
	"PR":  PR,
	"PO":  PO,
	"ID":  ID,
	"EB":  EB,
	"EM":  EM,
	"CB":  CB,
	"RI":  RI,
	"SA":  SA,
	"HL":  HL,
	"CJ":  CJ,
	"AK":  AK,
	"AP":  AP,
	"AS":  AS,
	"VF":  VF,
	"VI":  VI,
	"CP":  CP,
	"AI":  AI,
	"SG":  XX,
	"CM":  CM,
	"ZWJ": ZWJ,
	"H2":  H2,
	"H3":  H3,
	"JL":  JL,
	"JV":  JV,
	"JT":  JT,
	"HH":  HY,
}

// lb9XX maps each non-excluded base property to its _XX absorption state.
// Used only during generation by expandAll() and the LB9 absorption loop.
var lb9XX = map[uint8]uint8{
	XX:               XX_XX,
	AI:               AI_XX,
	AK:               AK_XX,
	AL:               AL_XX,
	AL_DC: AL_DC_XX,
	AP:               AP_XX,
	AS:               AS_XX,
	B2:               B2_XX,
	BA:               BA_XX,
	BB:               BB_XX,
	CB:               CB_XX,
	CJ:               CJ_XX,
	CL:               CL_XX,
	CP:               CP_XX,
	EB:               EB_XX,
	EM:               EM_XX,
	EX:               EX_XX,
	GL:               GL_XX,
	H2:               H2_XX,
	H3:               H3_XX,
	HL:               HL_XX,
	HY:               HY_XX,
	ID:               ID_XX,
	ID_ExtPict:            ID_ExtPict_XX,
	IN:               IN_XX,
	IS:               IS_XX,
	JL:               JL_XX,
	JT:               JT_XX,
	JV:               JV_XX,
	NS:               NS_XX,
	NU:               NU_XX,
	OP_EA:            OP_EA_XX,
	OP:          OP_XX,
	PO:               PO_XX,
	PO_EA:           PO_EA_XX,
	PR:               PR_XX,
	PR_EA:           PR_EA_XX,
	QU:               QU_XX,
	QU_PF:            QU_PF_XX,
	QU_PI:            QU_PI_XX,
	RI:               RI_XX,
	SA:               SA_XX,
	SY:               SY_XX,
	VF:               VF_XX,
	VI:               VI_XX,
	WJ:               WJ_XX,
}

func main() {
	gen.Init()
	genTables()
}

// versionAtLeast reports whether the Unicode version string v is >= the given
// major.minor version. Versions are compared as "major.minor" (patch ignored).
func versionAtLeast(v string, major, minor int) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false
	}
	var maj, min int
	fmt.Sscanf(parts[0], "%d", &maj)
	fmt.Sscanf(parts[1], "%d", &min)
	return maj > major || (maj == major && min >= minor)
}

func genTables() {
	gen.Repackage("gen_trieval.go", "trieval.go", "line")

	if stride > 120 {
		log.Fatalf("stride %d exceeds intermediateOffset 120", stride)
	}

	// =====================================================================
	// Parse UCD files and build the property trie.
	// =====================================================================

	props := make([]uint8, unicode.MaxRune+1)

	ucd.Parse(gen.OpenUCDFile("LineBreak.txt"), func(parser *ucd.Parser) {
		r := parser.Rune(0)
		val := parser.String(1)
		cls, ok := lbMap[val]
		if !ok {
			log.Fatalf("U+%04X: unknown Line_Break value %q", r, val)
		}
		props[r] = cls
	})

	eaw := make([]byte, unicode.MaxRune+1)
	ucd.Parse(gen.OpenUCDFile("EastAsianWidth.txt"), func(parser *ucd.Parser) {
		r := parser.Rune(0)
		val := parser.String(1)
		if len(val) > 0 {
			eaw[r] = val[0]
		}
	})

	gc := make([]string, unicode.MaxRune+1)
	ucd.Parse(gen.OpenUCDFile("UnicodeData.txt"), func(parser *ucd.Parser) {
		r := parser.Rune(0)
		gc[r] = parser.String(ucd.GeneralCategory)
	})

	extPictSet := make(map[rune]bool)
	ucd.Parse(gen.OpenUCDFile("emoji/emoji-data.txt"), func(parser *ucd.Parser) {
		if parser.String(1) == "Extended_Pictographic" {
			extPictSet[parser.Rune(0)] = true
		}
	})

	for r := rune(0); r <= unicode.MaxRune; r++ {
		isEA := eaw[r] == 'F' || eaw[r] == 'H' || eaw[r] == 'W'
		switch props[r] {
		case OP:
			if isEA {
				props[r] = OP_EA
			}
		case QU:
			switch gc[r] {
			case "Pi":
				props[r] = QU_PI
			case "Pf":
				props[r] = QU_PF
			}
		case SA:
			if gc[r] == "Mn" || gc[r] == "Mc" {
				props[r] = CM
			}
		case PR:
			if isEA {
				props[r] = PR_EA
			}
		case PO:
			if isEA {
				props[r] = PO_EA
			}
		case AL:
			if r == 0x25CC {
				props[r] = AL_DC
			}
		case ID:
			if (gc[r] == "Cn" || gc[r] == "") && extPictSet[r] {
				props[r] = ID_ExtPict
			}
		}
	}

	ucd.Parse(gen.OpenUCDFile("auxiliary/GraphemeBreakProperty.txt"), func(parser *ucd.Parser) {
		r := parser.Rune(0)
		val := parser.String(1)
		switch val {
		case "Extend":
			if props[r] == XX {
				props[r] = CM
			}
		case "ZWJ":
			props[r] = ZWJ
		}
	})

	// =====================================================================
	// Write output files.
	// =====================================================================

	w := gen.NewCodeWriter()
	defer w.WriteVersionedGoFile("tables.go", "line")

	fmt.Fprintf(w, "import %q\n\n", "golang.org/x/text/internal/segmenter")
	gen.WriteUnicodeVersion(w)

	t := triegen.NewTrie("line")
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if props[r] != 0 {
			t.Insert(r, uint64(props[r]))
		}
	}
	sz, err := t.Gen(w)
	if err != nil {
		log.Fatal(err)
	}
	w.Size += sz

	rules := buildRules(gen.UnicodeVersion())
	bt := segmenter.Build(rules, stride, sot, eot, lastCP)
	segmenter.WriteBreakTable(w, "ruleData", bt, "lineTrie", SA)
}

func p(v ...uint8) []uint8 { return v }

// expand returns the given base properties plus their LB9 _XX absorption
// states. This is the line-break equivalent of word's "ahletterPlusZWJ" groups.
func expand(props ...uint8) []uint8 {
	var r []uint8
	for _, base := range props {
		r = append(r, base)
		if xx, ok := lb9XX[base]; ok {
			r = append(r, xx)
		}
	}
	return r
}

func buildRules(unicodeVersion string) []segmenter.Rule {
	hasLB15b := versionAtLeast(unicodeVersion, 15, 1)

	idx := segmenter.IndexState
	interm := segmenter.IntermediateState

	allAlpha := expand(AI, AL, HL, XX, SA, CM, ZWJ)
	allAlphaTarget := expand(AI, AL, AL_DC, HL, XX, SA, CM, ZWJ)
	prAll := expand(PR, PR_EA)
	poAll := expand(PO, PO_EA)
	opAll := expand(OP, OP_EA)

	prpo := append(prAll, poAll...)
	mandatory := p(BK, CR, LF, NL)
	clcpexissy := expand(CL, CP, EX, IS, SY)
	quAll := expand(QU, QU_PF, QU_PI)
	bahyns := expand(BA, HY, NS, CJ)
	hangul := expand(JL, JV, JT, H2, H3)

	var rules []segmenter.Rule

	// =========================================================================
	// Rules in spec order (LB1–LB31). First-write-wins: earlier rules
	// (higher priority) take precedence.
	// =========================================================================

	// LB1: Assign a line breaking class to each code point of the input.
	// (Resolved at parse time: SA+Mn/Mc→CM. AI and CJ are stored as distinct properties.)

	// LB2: sot ×
	rules = append(rules, segmenter.SimpleRule{Left: p(sot), Break: false})

	// LB3: ! eot
	rules = append(rules, segmenter.SimpleRule{Right: p(eot), Break: true})

	// LB4: BK !
	rules = append(rules, segmenter.SimpleRule{Left: expand(BK), Break: true})

	// LB5: CR × LF, CR !, LF !, NL !
	rules = append(rules, segmenter.SimpleRule{Left: expand(CR), Right: p(LF), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(CR, LF, NL), Break: true})

	// LB6: × (BK | CR | LF | NL)
	rules = append(rules, segmenter.SimpleRule{Right: mandatory, Break: false})
	rules = append(rules, segmenter.SimpleRule{
		Left: p(B2_SP, CL_CP_SP, HL_HY, OP_SP, QU_SP, RI_RI, AK_VI), Right: mandatory, Break: false,
	})

	// LB7: × SP, × ZW
	rules = append(rules, segmenter.SimpleRule{Right: p(SP, ZW), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(HL_HY, RI_RI, AK_VI), Right: p(SP), Break: false})
	rules = append(rules, segmenter.SimpleRule{
		Left: p(B2_SP, CL_CP_SP, HL_HY, OP_SP, QU_SP, RI_RI, AK_VI), Right: p(ZW), Break: false,
	})

	// LB8: ZW SP* ÷
	rules = append(rules, segmenter.SimpleRule{Left: expand(ZW), Break: true})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(ZW),
		Steps: []segmenter.ChainStep{{Props: p(SP), State: ZW}},
	})

	// LB8a: ZWJ ×
	rules = append(rules, segmenter.SimpleRule{Left: expand(ZWJ), Break: false})

	// LB9: X (CM | ZWJ)* → X
	for base, xx := range lb9XX {
		rules = append(rules, segmenter.IgnoreRule{
			Props:   p(base, xx),
			Ignored: p(CM, ZWJ),
			Target: func(_, ign uint8) uint8 {
				if ign == ZWJ {
					return ZWJ_absorb
				}
				return xx
			},
		})
	}
	rules = append(rules, segmenter.OverrideRule{
		States:    p(ZWJ_absorb),
		WipeValue: segmenter.Keep,
		Overrides: map[uint8]uint8{
			eot: segmenter.Break,
			CM:  idx(ZWJ_absorb),
			ZWJ: idx(ZWJ_absorb),
		},
	})

	// LB10: Treat any remaining CM or ZWJ as AL.
	// (Handled by including CM/ZWJ in allAlpha groups.)

	// LB11: × WJ, WJ ×
	rules = append(rules, segmenter.SimpleRule{Right: expand(WJ), Break: false})
	rules = append(rules, segmenter.SimpleRule{
		Left: p(B2_SP, CL_CP_SP, HL_HY, OP_SP, QU_SP, RI_RI, AK_VI), Right: expand(WJ), Break: false,
	})
	rules = append(rules, segmenter.SimpleRule{Left: expand(WJ), Break: false})

	// LB12: GL ×
	rules = append(rules, segmenter.SimpleRule{Left: expand(GL), Break: false})

	// LB12a: [^SP BA HY] × GL
	rules = append(rules, segmenter.SimpleRule{
		Left: append(expand(SP, BA, HY), B2_SP, CL_CP_SP), Right: expand(GL), Break: true,
	})
	rules = append(rules, segmenter.SimpleRule{Right: expand(GL), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(HL_HY, OP_SP, RI_RI, AK_VI), Right: expand(GL), Break: false})

	// LB13: × CL, × CP, × EX, × IS, × SY
	rules = append(rules, segmenter.SimpleRule{Right: clcpexissy, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(B2_SP, CL_CP_SP, QU_SP, AK_VI, RI_RI), Right: clcpexissy, Break: false})

	// LB14: OP SP* ×
	rules = append(rules, segmenter.SimpleRule{Left: append(opAll, OP_SP), Break: false})
	rules = append(rules, segmenter.ChainRule{
		Entry: opAll,
		Steps: []segmenter.ChainStep{{Props: p(SP), State: OP_SP}},
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: p(OP_SP),
		Steps: []segmenter.ChainStep{{Props: p(SP), State: OP_SP}},
	})

	// LB15a: (sot | BK | CR | LF | NL | OP | QU | GL | SP | ZW) QU_PI SP* ×
	rules = append(rules, segmenter.ChainRule{
		Entry:  expand(QU_PI),
		Steps:  []segmenter.ChainStep{{Props: p(SP), State: QU_SP}},
		Interm: true,
	})
	if !hasLB15b {
		rules = append(rules, segmenter.ChainRule{
			Entry:  expand(QU, QU_PF),
			Steps:  []segmenter.ChainStep{{Props: p(SP), State: QU_SP}},
			Interm: true,
		})
	}
	var quPILeft []uint8
	for bp := uint8(0); bp <= lastBaseProperty; bp++ {
		if bp == BK || bp == CR || bp == LF || bp == NL || bp == SP || bp == ZW {
			continue
		}
		if bp == QU || bp == QU_PI || bp == QU_PF {
			continue
		}
		quPILeft = append(quPILeft, expand(bp)...)
	}
	quPILeft = append(quPILeft, HL_HY, AK_VI, AK_DC, RI_RI)
	rules = append(rules, segmenter.ChainRule{
		Entry: quPILeft,
		Steps: []segmenter.ChainStep{{Props: p(QU_PI), State: QU}},
	})
	rules = append(rules, segmenter.OverrideRule{
		States:    p(QU_SP),
		WipeValue: segmenter.NoMatch,
		Overrides: func() map[uint8]uint8 {
			m := map[uint8]uint8{
				eot: segmenter.Break,
				SP:  interm(QU_SP),
			}
			for _, op := range opAll {
				m[op] = segmenter.Keep
			}
			for _, mb := range mandatory {
				m[mb] = segmenter.Keep
			}
			m[ZW] = segmenter.Keep
			for _, c := range clcpexissy {
				m[c] = segmenter.Keep
			}
			for _, w := range expand(WJ) {
				m[w] = segmenter.Keep
			}
			return m
		}(),
	})

	// LB15b: × QU_PF (SP | GL | WJ | CL | QU | CP | EX | IS | SY | BK | CR | LF | NL | ZW | eot)
	rules = append(rules, segmenter.SimpleRule{
		Left:  append(expand(QU_PF), SP_QU, CB_QU),
		Right: append(p(SP, GL, WJ, CL, CP, EX, IS, SY, BK, CR, LF, NL, ZW), expand(QU, QU_PI, QU_PF)...),
		Break: false,
	})
	rules = append(rules, segmenter.SimpleRule{Left: append(expand(QU_PF), SP_QU, CB_QU), Right: p(eot), Break: true})
	if hasLB15b {
		rules = append(rules, segmenter.ChainRule{
			Entry: expand(SP),
			Steps: []segmenter.ChainStep{{Props: p(QU_PF), State: SP_QU}},
		})
		rules = append(rules, segmenter.ChainRule{
			Entry: p(B2_SP), Steps: []segmenter.ChainStep{{Props: p(QU_PF), State: SP_QU}}, Interm: true,
		})
		rules = append(rules, segmenter.ChainRule{
			Entry: p(CL_CP_SP), Steps: []segmenter.ChainStep{{Props: p(QU_PF), State: SP_QU}}, Interm: true,
		})
		rules = append(rules, segmenter.ChainRule{
			Entry: expand(CB),
			Steps: []segmenter.ChainStep{{Props: p(QU_PF), State: CB_QU}},
		})
		rules = append(rules, segmenter.ChainRule{
			Entry: p(OP_SP),
			Steps: []segmenter.ChainStep{{Props: p(QU_PF), State: QU_PF}},
		})
	}

	// LB16: (CL | CP) SP* × NS
	rules = append(rules, segmenter.SimpleRule{Left: append(expand(CL, CP), CL_CP_SP), Right: expand(NS, CJ), Break: false})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(CL, CP),
		Steps: []segmenter.ChainStep{{Props: p(SP), State: CL_CP_SP}},
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: p(CL_CP_SP),
		Steps: []segmenter.ChainStep{{Props: p(SP), State: CL_CP_SP}},
	})

	// LB17: B2 SP* × B2
	rules = append(rules, segmenter.SimpleRule{Left: append(expand(B2), B2_SP), Right: expand(B2), Break: false})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(B2),
		Steps: []segmenter.ChainStep{{Props: p(SP), State: B2_SP}},
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: p(B2_SP),
		Steps: []segmenter.ChainStep{{Props: p(SP), State: B2_SP}},
	})

	// LB18: SP ÷
	rules = append(rules, segmenter.SimpleRule{Left: expand(SP), Break: true})
	rules = append(rules, segmenter.SimpleRule{Left: p(B2_SP, CL_CP_SP), Break: true})

	// LB19: × QU, QU ×
	rules = append(rules, segmenter.SimpleRule{Right: quAll, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(RI_RI, AK_VI), Right: quAll, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: quAll, Break: false})

	// LB20: ÷ CB, CB ÷
	rules = append(rules, segmenter.SimpleRule{Left: expand(CB), Break: true})
	rules = append(rules, segmenter.SimpleRule{Right: expand(CB), Break: true})
	rules = append(rules, segmenter.SimpleRule{Left: p(HL_HY), Right: expand(CB), Break: true})
	rules = append(rules, segmenter.SimpleRule{Left: p(CB_QU), Break: false})

	// LB21: × BA, × HY, × NS, BB ×
	rules = append(rules, segmenter.SimpleRule{Right: bahyns, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(RI_RI, AK_VI), Right: bahyns, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(BB), Break: false})

	// LB21a: HL (HY | BA) ×
	rules = append(rules, segmenter.SimpleRule{Left: p(HL_HY), Break: false})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(HL),
		Steps: []segmenter.ChainStep{{Props: expand(HY, BA), State: HL_HY}},
	})

	// LB21b: SY × HL
	rules = append(rules, segmenter.SimpleRule{Left: expand(SY), Right: expand(HL), Break: false})

	// LB22: × IN
	rules = append(rules, segmenter.SimpleRule{Right: expand(IN), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(RI_RI, AK_VI), Right: expand(IN), Break: false})

	// LB23: (AL | HL) × NU, NU × (AL | HL)
	rules = append(rules, segmenter.SimpleRule{Left: allAlpha, Right: expand(NU), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(NU), Right: allAlphaTarget, Break: false})

	// LB23a: PR × (ID | EB | EM), (ID | EB | EM) × PO
	rules = append(rules, segmenter.SimpleRule{Left: prAll, Right: expand(ID, ID_ExtPict, EB, EM), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(ID, ID_ExtPict, EB, EM), Right: poAll, Break: false})

	// LB24: (PR | PO) × (AL | HL), (AL | HL) × (PR | PO)
	rules = append(rules, segmenter.SimpleRule{Left: prpo, Right: allAlphaTarget, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: allAlpha, Right: prpo, Break: false})

	// LB25: Numeric context (NU (SY|IS)* (CL|CP)? (PR|PO)?, PR|PO × OP? NU, HY × NU)
	rules = append(rules, segmenter.SimpleRule{Left: expand(HY), Right: expand(NU), Break: false})

	lb25Entry := append(prAll, poAll...)

	rules = append(rules, segmenter.ChainRule{
		Entry: lb25Entry,
		Steps: []segmenter.ChainStep{{Props: opAll, State: NU_OP}},
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: lb25Entry, Steps: []segmenter.ChainStep{{Props: expand(NU), State: NU_Num}}, Interm: true,
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: opAll, Steps: []segmenter.ChainStep{{Props: expand(NU), State: NU_Num}}, Interm: true,
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(HY), Steps: []segmenter.ChainStep{{Props: expand(NU), State: NU_Num}}, Interm: true,
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(NU), Steps: []segmenter.ChainStep{{Props: expand(NU, SY, IS), State: NU_Num}}, Interm: true,
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(NU), Steps: []segmenter.ChainStep{{Props: expand(CL), State: NU_Close_CL}}, Interm: true,
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(NU), Steps: []segmenter.ChainStep{{Props: expand(CP), State: NU_Close_CP}}, Interm: true,
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(NU), Steps: []segmenter.ChainStep{{Props: lb25Entry, State: NU_Post}}, Interm: true,
	})

	nuCommonKeep := func(m map[uint8]uint8) {
		for _, r := range mandatory {
			m[r] = segmenter.Keep
		}
		for _, r := range p(SP, ZW) {
			m[r] = segmenter.Keep
		}
		for _, r := range expand(WJ) {
			m[r] = segmenter.Keep
		}
		for _, r := range clcpexissy {
			m[r] = segmenter.Keep
		}
		for _, r := range quAll {
			m[r] = segmenter.Keep
		}
	}

	rules = append(rules, segmenter.OverrideRule{
		States: p(NU_OP), WipeValue: segmenter.NoMatch,
		Overrides: func() map[uint8]uint8 {
			m := make(map[uint8]uint8)
			for _, nu := range expand(NU) {
				m[nu] = interm(NU_Num)
			}
			return m
		}(),
	})
	rules = append(rules, segmenter.OverrideRule{
		States: p(NU_Num), WipeValue: segmenter.NoMatch,
		Overrides: func() map[uint8]uint8 {
			m := map[uint8]uint8{eot: segmenter.Break}
			nuCommonKeep(m)
			for _, r := range expand(IN) {
				m[r] = segmenter.Keep
			}
			for _, r := range bahyns {
				m[r] = segmenter.Keep
			}
			for _, r := range allAlphaTarget {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(GL) {
				m[r] = segmenter.Keep
			}
			for _, r := range opAll {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(EX) {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(NU, SY, IS) {
				m[r] = interm(NU_Num)
			}
			for _, r := range expand(CL) {
				m[r] = interm(NU_Close_CL)
			}
			for _, r := range expand(CP) {
				m[r] = interm(NU_Close_CP)
			}
			for _, r := range lb25Entry {
				m[r] = interm(NU_Post)
			}
			return m
		}(),
	})
	rules = append(rules, segmenter.OverrideRule{
		States: p(NU_Close_CL), WipeValue: segmenter.NoMatch,
		Overrides: func() map[uint8]uint8 {
			m := map[uint8]uint8{eot: segmenter.Break}
			for _, r := range lb25Entry {
				m[r] = interm(NU_Post)
			}
			nuCommonKeep(m)
			for _, r := range expand(IN) {
				m[r] = segmenter.Keep
			}
			for _, r := range bahyns {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(GL) {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(BB) {
				m[r] = segmenter.Keep
			}
			return m
		}(),
	})
	rules = append(rules, segmenter.OverrideRule{
		States: p(NU_Close_CP), WipeValue: segmenter.NoMatch,
		Overrides: func() map[uint8]uint8 {
			m := map[uint8]uint8{eot: segmenter.Break}
			for _, r := range lb25Entry {
				m[r] = interm(NU_Post)
			}
			nuCommonKeep(m)
			for _, r := range allAlphaTarget {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(NU) {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(IN) {
				m[r] = segmenter.Keep
			}
			for _, r := range bahyns {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(GL) {
				m[r] = segmenter.Keep
			}
			for _, r := range expand(BB) {
				m[r] = segmenter.Keep
			}
			return m
		}(),
	})
	rules = append(rules, segmenter.OverrideRule{
		States: p(NU_Post), WipeValue: segmenter.NoMatch,
		Overrides: func() map[uint8]uint8 {
			m := map[uint8]uint8{eot: segmenter.Break}
			for _, r := range opAll {
				m[r] = idx(NU_OP)
			}
			for _, r := range expand(NU) {
				m[r] = interm(NU_Num)
			}
			nuCommonKeep(m)
			for _, r := range expand(HY) {
				m[r] = segmenter.Keep
			}
			return m
		}(),
	})

	// LB26: JL × (JL | JV | H2 | H3), (JV | H2) × (JV | JT), (JT | H3) × JT
	rules = append(rules, segmenter.SimpleRule{Left: expand(JL), Right: expand(JL, JV, H2, H3), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(JV, H2), Right: expand(JV, JT), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(JT, H3), Right: expand(JT), Break: false})

	// LB27: (JL | JV | JT | H2 | H3) × PO, PR × (JL | JV | JT | H2 | H3)
	rules = append(rules, segmenter.SimpleRule{Left: hangul, Right: poAll, Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: prAll, Right: hangul, Break: false})

	// LB28: (AL | HL) × (AL | HL)
	rules = append(rules, segmenter.SimpleRule{Left: allAlpha, Right: allAlphaTarget, Break: false})

	// LB28a: AP × (AK | ◌ | AS), (AK | ◌ | AS) × (VF | VI), (AK | ◌ | AS) VI × (AK | ◌), (AK | ◌ | AS) × (AK | ◌ | AS) VF
	rules = append(rules, segmenter.SimpleRule{Left: append(expand(AL_DC), AK_DC), Right: expand(AI, AL, HL, XX, SA), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(AP), Right: expand(AK, AL_DC, AS), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: expand(AK, AL_DC, AS), Right: expand(VF), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(AK_VI), Right: expand(AK, AL_DC), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(AK_AK, AK_DC), Right: expand(VF), Break: false})
	rules = append(rules, segmenter.SimpleRule{Left: p(AK_VI), Break: true})
	rules = append(rules, segmenter.SimpleRule{Left: p(AK_DC), Break: true})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(AK, AL_DC, AS),
		Steps: []segmenter.ChainStep{{Props: expand(VI), State: AK_VI}},
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(AK, AL_DC, AS),
		Steps: []segmenter.ChainStep{{Props: expand(AK, AS), State: AK_AK}},
	})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(AL_DC),
		Steps: []segmenter.ChainStep{{Props: expand(AL_DC), State: AK_DC}},
	})

	// LB29: IS × (AL | HL)
	rules = append(rules, segmenter.SimpleRule{Left: expand(IS), Right: expand(AI, AL, HL, SA, XX, AL_DC), Break: false})

	// LB30: (AL | HL | NU) × OP_nonEA, CP_nonEA × (AL | HL | NU)
	rules = append(rules, segmenter.SimpleRule{
		Left:  append(append(allAlpha, expand(NU, AL_DC)...), AK_DC),
		Right: expand(OP),
		Break: false,
	})
	rules = append(rules, segmenter.SimpleRule{Left: expand(CP), Right: append(allAlphaTarget, expand(NU)...), Break: false})

	// LB30a: sot (RI RI)* RI × RI, [^RI] (RI RI)* RI × RI
	rules = append(rules, segmenter.SimpleRule{Left: p(RI_RI), Right: expand(RI), Break: true})
	rules = append(rules, segmenter.SimpleRule{Left: p(RI_RI), Break: true})
	rules = append(rules, segmenter.ChainRule{
		Entry: expand(RI),
		Steps: []segmenter.ChainStep{{Props: expand(RI), State: RI_RI}},
	})

	// LB30b: EB × EM, [\p{Extended_Pictographic}&\p{Cn}] × EM
	rules = append(rules, segmenter.SimpleRule{Left: expand(EB, ID_ExtPict), Right: expand(EM), Break: false})

	// LB31: ALL ÷
	rules = append(rules, segmenter.SimpleRule{Break: true})

	return rules
}
