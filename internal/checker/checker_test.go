package checker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidProgram(t *testing.T) {
	src := `USEVAR COUNT
COUNT=0
FOR I=0 TO 10
 COUNT=COUNT+I
NEXT
GOSUB SUB1
END
LABEL SUB1
RETURN
`
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("unexpected errors: %#v", result.Diagnostics)
	}
}

func TestStructuralErrors(t *testing.T) {
	src := `FOR I#=0 TO 10
IF A=1 THEN
NEXT
GOTO MISSING
`
	result := checkTemp(t, src)
	if result.ErrorCount() < 3 {
		t.Fatalf("expected at least 3 errors: %#v", result.Diagnostics)
	}
}

func TestReservedVariable(t *testing.T) {
	result := checkTemp(t, "USEVAR CLEAR\n")
	if result.ErrorCount() == 0 {
		t.Fatalf("reserved word was not detected: %#v", result.Diagnostics)
	}
}

func TestFunctionNameIsReserved(t *testing.T) {
	result := checkTemp(t, "USEVAR SIN\n")
	assertDiagnostic(t, result, "KM3003")
}

func checkTemp(t *testing.T, src string) FileResult {
	return checkTempWithOptions(t, src, Options{Target: "type-pu"})
}

func checkTempWithOptions(t *testing.T, src string, options Options) FileResult {
	t.Helper()
	dir := t.TempDir()
	name := filepath.Join(dir, "TEST.BAS")
	if err := os.WriteFile(name, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CheckFile(name, options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDimArray(t *testing.T) {
	src := `USEVAR table_,names
DIM d(2)
DIM table_(10,20), names$(5)
`
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("valid DIM declarations caused errors: %#v", result.Diagnostics)
	}
}

func TestDimInvalidDeclaration(t *testing.T) {
	result := checkTemp(t, "DIM d()\n")
	if result.ErrorCount() == 0 {
		t.Fatalf("empty DIM size was not detected: %#v", result.Diagnostics)
	}
}

func TestUnknownStatement(t *testing.T) {
	result := checkTemp(t, "PRONT 123\n")
	assertDiagnostic(t, result, "KM4001")
}

func TestUnknownFunction(t *testing.T) {
	result := checkTemp(t, "A=SQRRT#(4)\n")
	assertDiagnostic(t, result, "KM4002")
}

func TestGuideCommandsAndFunctionsAreKnown(t *testing.T) {
	src := "USEGRAPHIC 1\nGCLS\nPSET 10,20,GCOLOR(10,20)\nA#=SIN#(PI#)\nB=PEEK(100)\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("guide names caused errors: %#v", result.Diagnostics)
	}
}

func TestArrayAndStringSliceAreNotFunctions(t *testing.T) {
	src := "USEVAR TABLE,NAME\nDIM TABLE(10)\nA=TABLE(1)\nB$=NAME$(2,3)\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("variables were mistaken for functions: %#v", result.Diagnostics)
	}
}

func assertDiagnostic(t *testing.T, result FileResult, code string) {
	t.Helper()
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("diagnostic %s not found: %#v", code, result.Diagnostics)
}

func TestAssignmentTypeChecking(t *testing.T) {
	src := "A=1.5\nC$=123\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5002"); got != 2 {
		t.Fatalf("expected 2 assignment errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestExpressionOperatorTypes(t *testing.T) {
	src := "B$=\"x\"-\"y\"\nC#=2.0%1.0\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5003"); got != 2 {
		t.Fatalf("expected 2 operator errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestIntegerPromotesToReal(t *testing.T) {
	src := "A#=3\nA#=A# / 4\nB#=1+2.5\nC#=2*3.0+1\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("integer-to-real promotion caused errors: %#v", result.Diagnostics)
	}
}

func TestRealDoesNotDemoteToInteger(t *testing.T) {
	result := checkTemp(t, "A=3.0\n")
	assertDiagnostic(t, result, "KM5002")
}

func TestLogicalOperatorsAreNotVariables(t *testing.T) {
	src := "A=1 AND 2\nB=1 OR 2\nC=1 XOR 2\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM3007"); got != 0 {
		t.Fatalf("logical operators were mistaken for variables: %#v", result.Diagnostics)
	}
}

func TestFunctionArgumentTypes(t *testing.T) {
	src := "A=INT(1)\nB#=SIN#(1)\nC=LEN(10)\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5004"); got != 3 {
		t.Fatalf("expected 3 function argument errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestRealRandomFunction(t *testing.T) {
	src := "A=RND()\nB#=RND#()\nC#=RND#()*10\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("RND# caused errors: %#v", result.Diagnostics)
	}
}

func TestRealRandomFunctionRejectsArguments(t *testing.T) {
	result := checkTemp(t, "A#=RND#(1)\n")
	assertDiagnostic(t, result, "KM5004")
}

func TestValidTypedExpressions(t *testing.T) {
	src := "A=INT(1.5)+2\nB#=SIN#(FLOAT#(A))*PI#\nC$=DEC$(A)+\"!\"\nIF C$!=\"0\" THEN\nENDIF\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("valid typed expressions caused errors: %#v", result.Diagnostics)
	}
}

func TestArrayIndexAndDimensionTypes(t *testing.T) {
	src := "DIM A#(10)\nDIM B(2.5)\nA#(1.0)=2.0\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5005"); got != 2 {
		t.Fatalf("expected 2 array index errors, got %d: %#v", got, result.Diagnostics)
	}
}

func countDiagnostic(result FileResult, code string) int {
	n := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			n++
		}
	}
	return n
}

func TestREMConsumesRestOfStatementLine(t *testing.T) {
	src := "REM comment: PRONT 1: \"unclosed\nA=1: REM comment: SQRRT#(1) \"\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("REM body was checked as code: %#v", result.Diagnostics)
	}
}

func TestREMRequiresStatementStart(t *testing.T) {
	result := checkTemp(t, "USEVAR REMAIN\nA=REMAIN+1\n")
	if result.ErrorCount() != 0 {
		t.Fatalf("identifier beginning with REM was treated as a comment: %#v", result.Diagnostics)
	}
}

func TestApostropheIsNotAComment(t *testing.T) {
	result := checkTemp(t, "A=1 ' comment\nREM apostrophe ' is harmless here\nA$=\"'\"\n")
	if got := countDiagnostic(result, "KM0002"); got != 1 {
		t.Fatalf("expected one apostrophe diagnostic, got %d: %#v", got, result.Diagnostics)
	}
	if result.ErrorCount() != 1 {
		t.Fatalf("apostrophe caused cascading diagnostics: %#v", result.Diagnostics)
	}
}

func TestOperatorPrecedenceMatchesGuide(t *testing.T) {
	ordered := []string{"OR", "AND", "XOR", "=", "<", "<<", "+", "*"}
	for i := 1; i < len(ordered); i++ {
		if operatorPrecedence(ordered[i-1]) >= operatorPrecedence(ordered[i]) {
			t.Fatalf("operator precedence is wrong: %s, %s", ordered[i-1], ordered[i])
		}
	}
}

func TestOperatorTypeMatrix(t *testing.T) {
	src := "A$=\"a\"%\"b\"\nA#=1.0 XOR 2.0\nA=1 AND 2\nA$=\"a\" OR \"b\"\nA=&(1+2)\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5003"); got != 3 {
		t.Fatalf("expected 3 invalid operator diagnostics, got %d: %#v", got, result.Diagnostics)
	}
}

func TestLongVariableRequiresUseVar(t *testing.T) {
	src := "COUNT=1\nPRINT TOTAL\nUSEVAR COUNT,TOTAL\nCOUNT#=1.0\nTOTAL$=\"ok\"\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM3007"); got != 2 {
		t.Fatalf("expected 2 undeclared long variables, got %d: %#v", got, result.Diagnostics)
	}
}

func TestUseVarAppliesToAllVariableTypes(t *testing.T) {
	src := "USEVAR VALUE\nVALUE=1\nVALUE#=1.0\nVALUE$=\"x\"\nDIM VALUE(10)\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("declared variable caused errors: %#v", result.Diagnostics)
	}
}

func TestLongDimAndVarRequireUseVar(t *testing.T) {
	src := "DIM TABLE(10)\nVAR LOCAL#\nUSEVAR TABLE,LOCAL\nDIM TABLE$(10)\nVAR LOCAL$\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM3007"); got != 2 {
		t.Fatalf("expected DIM and VAR declaration errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestLabelsAndFunctionsAreNotLongVariables(t *testing.T) {
	src := "GOTO TARGET\nA=INKEY()\nLABEL TARGET\nEND\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("labels or functions were mistaken for variables: %#v", result.Diagnostics)
	}
}

func TestStatementArgumentCount(t *testing.T) {
	src := "COLOR\nCURSOR 1\nPALETTE 1,2,3,4,5\nFOPEN \"A.TXT\"\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5101"); got != 4 {
		t.Fatalf("expected 4 argument-count errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestStatementArgumentTypes(t *testing.T) {
	src := "COLOR \"red\"\nSETTIME 123\nFOPEN 1,\"r\",1\nGPRINT 1,2,3,4,5\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5102"); got != 4 {
		t.Fatalf("expected 4 argument-type errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestOptionalAndVariadicStatementArguments(t *testing.T) {
	src := "FCLOSE\nFOPEN \"A.TXT\",\"r\"\nUSEPCG\nPWM 100\nSPIWRITE 1,2,3\nI2CWRITE $50,0,1\nDATA 1,2.0,\"x\"\nRETURN \"done\"\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("valid optional or variadic arguments caused errors: %#v", result.Diagnostics)
	}
}

func TestPrintArguments(t *testing.T) {
	src := "PRINT 1;2.0;\"x\"\nFPRINT \"A=\";A\nPRINT SIN#(1)\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5004"); got != 1 {
		t.Fatalf("PRINT expression was not checked: %#v", result.Diagnostics)
	}
}

func TestOmittedGraphicCoordinates(t *testing.T) {
	src := "BOXFILL ,10,20\nCIRCLE ,50\nLINE ,100,200\nGPRINT ,7,-1,\"DEF\"\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("omitted graphic coordinates caused errors: %#v", result.Diagnostics)
	}
}

func TestInlineIfStatementArguments(t *testing.T) {
	src := "IF A THEN COLOR \"red\" ELSE WAIT 1.0\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5102"); got != 2 {
		t.Fatalf("expected inline IF argument errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestParenthesizedIfConditionIsNotAFunction(t *testing.T) {
	src := "USEVAR DX,DY\nIF (DX!=0) OR (DY!=0) THEN\nENDIF\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("IF with a parenthesized condition was mistaken for a function: %#v", result.Diagnostics)
	}
}

func TestLabelCommandArgumentCounts(t *testing.T) {
	src := "GOTO\nRESTORE A,B\nSOUND\nGOSUB SUB,1,\"x\"\nEND\nLABEL SUB\nRETURN\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5101"); got != 3 {
		t.Fatalf("expected 3 label-command count errors, got %d: %#v", got, result.Diagnostics)
	}
}

func TestFGetStatementArguments(t *testing.T) {
	src := "DIM B(10)\nFGET B,256\nFGET B,1.5\n"
	result := checkTemp(t, src)
	if got := countDiagnostic(result, "KM5102"); got != 1 {
		t.Fatalf("FGET arguments were not checked: %#v", result.Diagnostics)
	}
}

func TestPutBMPAcceptsArrayOrLabel(t *testing.T) {
	src := "USEVAR PIXELS\nDIM PIXELS(6)\nPUTBMP 0,0,5,5,PIXELS\nPUTBMP 0,0,5,5,BITMAP_DATA\nLABEL BITMAP_DATA\nCDATA 1,2,3\n"
	result := checkTemp(t, src)
	if result.ErrorCount() != 0 {
		t.Fatalf("PUTBMP array or label caused errors: %#v", result.Diagnostics)
	}
}

func TestPutBMPRejectsInvalidDataArgument(t *testing.T) {
	result := checkTemp(t, "PUTBMP 0,0,5,5,A+1\n")
	assertDiagnostic(t, result, "KM5103")
}

func TestPutBMPReportsUndefinedLabel(t *testing.T) {
	result := checkTemp(t, "PUTBMP 0,0,5,5,MISSING_BITMAP\n")
	assertDiagnostic(t, result, "KM2001")
}

func TestClassLibraryUsage(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "LIB", "DEMO")
	if err := os.MkdirAll(lib, 0o700); err != nil {
		t.Fatal(err)
	}
	classSource := "FIELD PUBLIC VALUE,NAME$\nSTATIC PUBLIC COUNT\nMETHOD INIT\nMETHOD RUN\n"
	if err := os.WriteFile(filepath.Join(lib, "DEMO.BAS"), []byte(classSource), 0o600); err != nil {
		t.Fatal(err)
	}
	src := "USECLASS DEMO\nUSEVAR OBJ\nOBJ=NEW(DEMO)\nCALL OBJ.RUN(1)\nA=OBJ.VALUE\nB$=OBJ.NAME$\nC=DEMO::COUNT\n"
	result := checkTempWithOptions(t, src, Options{Target: "type-pu", LibraryDirs: []string{filepath.Dir(lib)}})
	if result.ErrorCount() != 0 {
		t.Fatalf("valid class-library usage caused errors: %#v", result.Diagnostics)
	}
}

func TestUnknownAndUnusedClass(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "LIB", "DEMO")
	if err := os.MkdirAll(lib, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "DEMO.BAS"), []byte("METHOD INIT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := Options{Target: "type-pu", LibraryDirs: []string{filepath.Dir(lib)}}
	unknown := checkTempWithOptions(t, "USECLASS MISSING\n", options)
	assertDiagnostic(t, unknown, "KM6001")
	unused := checkTempWithOptions(t, "A=NEW(DEMO)\n", options)
	assertDiagnostic(t, unused, "KM6002")
}
