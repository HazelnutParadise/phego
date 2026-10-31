// Package frontend turns PHP source into tokens and syntax trees for a
// selected release profile.
package frontend

// Kind identifies a token. Values below 256 are single-character tokens and
// equal that byte; larger values are the named tokens below.
type Kind int

const (
	TInlineHTML Kind = 256 + iota
	TOpenTag
	TOpenTagWithEcho
	TCloseTag
	TWhitespace
	TComment
	TDocComment
	TVariable
	TString
	TNameQualified
	TNameFullyQualified
	TNameRelative
	TNsSeparator
	TLNumber
	TDNumber
	TConstantEncapsedString
	TEncapsedAndWhitespace
	TStartHeredoc
	TEndHeredoc
	TCurlyOpen
	TDollarOpenCurlyBraces
	TStringVarname
	TNumString
	TObjectOperator
	TNullsafeObjectOperator
	TDoubleArrow
	TDoubleColon
	TInc
	TDec
	TIsEqual
	TIsNotEqual
	TIsIdentical
	TIsNotIdentical
	TIsSmallerOrEqual
	TIsGreaterOrEqual
	TSpaceship
	TPlusEqual
	TMinusEqual
	TMulEqual
	TDivEqual
	TConcatEqual
	TModEqual
	TAndEqual
	TOrEqual
	TXorEqual
	TSlEqual
	TSrEqual
	TPowEqual
	TCoalesceEqual
	TBooleanAnd
	TBooleanOr
	TCoalesce
	TSl
	TSr
	TPow
	TEllipsis
	TPipe
	TAttribute
	TIntCast
	TDoubleCast
	TStringCast
	TArrayCast
	TObjectCast
	TBoolCast
	TUnsetCast
	TVoidCast
	TYieldFrom
	TBadCharacter
	TAmpersandFollowedByVarOrVararg
	TAmpersandNotFollowedByVarOrVararg
	// Keywords.
	TAbstract
	TLogicalAnd
	TArray
	TAs
	TBreak
	TCallable
	TCase
	TCatch
	TClass
	TClone
	TConst
	TContinue
	TDeclare
	TDefault
	TDo
	TEcho
	TElse
	TElseif
	TEmpty
	TEnddeclare
	TEndfor
	TEndforeach
	TEndif
	TEndswitch
	TEndwhile
	TEnum
	TEval
	TExit
	TExtends
	TFinal
	TFinally
	TFn
	TFor
	TForeach
	TFunction
	TGlobal
	TGoto
	TIf
	TImplements
	TInclude
	TIncludeOnce
	TInstanceof
	TInsteadof
	TInterface
	TIsset
	TList
	TMatch
	TNamespace
	TNew
	TLogicalOr
	TPrint
	TPrivate
	TProtected
	TPublic
	TPrivateSet
	TProtectedSet
	TPublicSet
	TReadonly
	TRequire
	TRequireOnce
	TReturn
	TStatic
	TSwitch
	TThrow
	TTrait
	TTry
	TUnset
	TUse
	TVar
	TWhile
	TLogicalXor
	TYield
	TClassC
	TDir
	TFile
	TFuncC
	TLine
	TMethodC
	TNsC
	TTraitC
	TPropertyC
	THaltCompiler
	kindEnd
)

var kindNames = [...]string{
	"T_INLINE_HTML", "T_OPEN_TAG", "T_OPEN_TAG_WITH_ECHO", "T_CLOSE_TAG", "T_WHITESPACE",
	"T_COMMENT", "T_DOC_COMMENT", "T_VARIABLE", "T_STRING", "T_NAME_QUALIFIED",
	"T_NAME_FULLY_QUALIFIED", "T_NAME_RELATIVE", "T_NS_SEPARATOR", "T_LNUMBER", "T_DNUMBER",
	"T_CONSTANT_ENCAPSED_STRING", "T_ENCAPSED_AND_WHITESPACE", "T_START_HEREDOC", "T_END_HEREDOC",
	"T_CURLY_OPEN", "T_DOLLAR_OPEN_CURLY_BRACES", "T_STRING_VARNAME", "T_NUM_STRING",
	"T_OBJECT_OPERATOR", "T_NULLSAFE_OBJECT_OPERATOR", "T_DOUBLE_ARROW", "T_DOUBLE_COLON",
	"T_INC", "T_DEC", "T_IS_EQUAL", "T_IS_NOT_EQUAL", "T_IS_IDENTICAL", "T_IS_NOT_IDENTICAL",
	"T_IS_SMALLER_OR_EQUAL", "T_IS_GREATER_OR_EQUAL", "T_SPACESHIP", "T_PLUS_EQUAL",
	"T_MINUS_EQUAL", "T_MUL_EQUAL", "T_DIV_EQUAL", "T_CONCAT_EQUAL", "T_MOD_EQUAL",
	"T_AND_EQUAL", "T_OR_EQUAL", "T_XOR_EQUAL", "T_SL_EQUAL", "T_SR_EQUAL", "T_POW_EQUAL",
	"T_COALESCE_EQUAL", "T_BOOLEAN_AND", "T_BOOLEAN_OR", "T_COALESCE", "T_SL", "T_SR", "T_POW",
	"T_ELLIPSIS", "T_PIPE", "T_ATTRIBUTE", "T_INT_CAST", "T_DOUBLE_CAST", "T_STRING_CAST",
	"T_ARRAY_CAST", "T_OBJECT_CAST", "T_BOOL_CAST", "T_UNSET_CAST", "T_VOID_CAST",
	"T_YIELD_FROM", "T_BAD_CHARACTER", "T_AMPERSAND_FOLLOWED_BY_VAR_OR_VARARG",
	"T_AMPERSAND_NOT_FOLLOWED_BY_VAR_OR_VARARG",
	"T_ABSTRACT", "T_LOGICAL_AND", "T_ARRAY", "T_AS", "T_BREAK", "T_CALLABLE", "T_CASE",
	"T_CATCH", "T_CLASS", "T_CLONE", "T_CONST", "T_CONTINUE", "T_DECLARE", "T_DEFAULT", "T_DO",
	"T_ECHO", "T_ELSE", "T_ELSEIF", "T_EMPTY", "T_ENDDECLARE", "T_ENDFOR", "T_ENDFOREACH",
	"T_ENDIF", "T_ENDSWITCH", "T_ENDWHILE", "T_ENUM", "T_EVAL", "T_EXIT", "T_EXTENDS", "T_FINAL",
	"T_FINALLY", "T_FN", "T_FOR", "T_FOREACH", "T_FUNCTION", "T_GLOBAL", "T_GOTO", "T_IF",
	"T_IMPLEMENTS", "T_INCLUDE", "T_INCLUDE_ONCE", "T_INSTANCEOF", "T_INSTEADOF", "T_INTERFACE",
	"T_ISSET", "T_LIST", "T_MATCH", "T_NAMESPACE", "T_NEW", "T_LOGICAL_OR", "T_PRINT",
	"T_PRIVATE", "T_PROTECTED", "T_PUBLIC", "T_PRIVATE_SET", "T_PROTECTED_SET", "T_PUBLIC_SET",
	"T_READONLY", "T_REQUIRE", "T_REQUIRE_ONCE", "T_RETURN", "T_STATIC", "T_SWITCH", "T_THROW",
	"T_TRAIT", "T_TRY", "T_UNSET", "T_USE", "T_VAR", "T_WHILE", "T_LOGICAL_XOR", "T_YIELD",
	"T_CLASS_C", "T_DIR", "T_FILE", "T_FUNC_C", "T_LINE", "T_METHOD_C", "T_NS_C", "T_TRAIT_C",
	"T_PROPERTY_C", "T_HALT_COMPILER",
}

// String returns PHP's token_name() for named tokens and the character for
// single-character tokens.
func (k Kind) String() string {
	if k >= 0 && k < 256 {
		return string(rune(k))
	}
	if k >= TInlineHTML && k < kindEnd {
		return kindNames[k-TInlineHTML]
	}
	return "UNKNOWN"
}

var keywords = map[string]Kind{
	"abstract": TAbstract, "and": TLogicalAnd, "array": TArray, "as": TAs, "break": TBreak,
	"callable": TCallable, "case": TCase, "catch": TCatch, "class": TClass, "clone": TClone,
	"const": TConst, "continue": TContinue, "declare": TDeclare, "default": TDefault, "do": TDo,
	"echo": TEcho, "else": TElse, "elseif": TElseif, "empty": TEmpty, "enddeclare": TEnddeclare,
	"endfor": TEndfor, "endforeach": TEndforeach, "endif": TEndif, "endswitch": TEndswitch,
	"endwhile": TEndwhile, "eval": TEval, "exit": TExit, "die": TExit, "extends": TExtends,
	"final": TFinal, "finally": TFinally, "fn": TFn, "for": TFor, "foreach": TForeach,
	"function": TFunction, "global": TGlobal, "goto": TGoto, "if": TIf, "implements": TImplements,
	"include": TInclude, "include_once": TIncludeOnce, "instanceof": TInstanceof,
	"insteadof": TInsteadof, "interface": TInterface, "isset": TIsset, "list": TList,
	"match": TMatch, "namespace": TNamespace, "new": TNew, "or": TLogicalOr, "print": TPrint,
	"private": TPrivate, "protected": TProtected, "public": TPublic, "readonly": TReadonly,
	"require": TRequire, "require_once": TRequireOnce, "return": TReturn, "static": TStatic,
	"switch": TSwitch, "throw": TThrow, "trait": TTrait, "try": TTry, "unset": TUnset, "use": TUse,
	"var": TVar, "while": TWhile, "xor": TLogicalXor, "yield": TYield, "__class__": TClassC,
	"__dir__": TDir, "__file__": TFile, "__function__": TFuncC, "__line__": TLine,
	"__method__": TMethodC, "__namespace__": TNsC, "__trait__": TTraitC, "__property__": TPropertyC,
	"__halt_compiler": THaltCompiler,
}

// Token is one lexeme. Text aliases the source buffer.
type Token struct {
	Kind Kind
	Text string
	Line int
}

// Diagnostic is a located lexing problem. Lexing continues after it.
type Diagnostic struct {
	Line    int
	Message string
}
