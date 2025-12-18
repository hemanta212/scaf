package cypher

import (
	"strings"

	"github.com/rlch/scaf/analysis"
	cyphergrammar "github.com/rlch/scaf/dialects/cypher/grammar"
)

// ----------------------------------------------------------------------------
// Cypher Function Type Registry
//
// This file defines the return types for built-in Cypher functions.
// Functions are split into two categories:
// - Fixed return types (most functions)
// - Inference functions (for functions whose return type depends on arguments)
// ----------------------------------------------------------------------------

// cypherFunctionTypes maps function names (lowercase) to their fixed return types.
var cypherFunctionTypes = map[string]*analysis.Type{
	// ============================================================================
	// Neo4j Built-in Functions
	// ============================================================================

	// Aggregation functions
	"count":          analysis.TypeInt,
	"sum":            analysis.TypeFloat64,
	"avg":            analysis.TypeFloat64,
	"stdev":          analysis.TypeFloat64,
	"stdevp":         analysis.TypeFloat64,
	"variance":       analysis.TypeFloat64,
	"percentilecont": analysis.TypeFloat64,
	"percentiledisc": analysis.TypeFloat64,

	// Type conversion functions
	"tostring":        analysis.TypeString,
	"tostringornull":  analysis.TypeString,
	"toboolean":       analysis.TypeBool,
	"tobooleanornull": analysis.TypeBool,
	"tobool":          analysis.TypeBool,
	"tointeger":       analysis.TypeInt,
	"tointegerornull": analysis.TypeInt,
	"toint":           analysis.TypeInt,
	"tofloat":         analysis.TypeFloat64,
	"tofloatornull":   analysis.TypeFloat64,

	// List conversion functions
	"tobooleanlist": analysis.SliceOf(analysis.TypeBool),
	"tointegerlist": analysis.SliceOf(analysis.TypeInt),
	"tofloatlist":   analysis.SliceOf(analysis.TypeFloat64),
	"tostringlist":  analysis.SliceOf(analysis.TypeString),

	// Predicate functions
	"exists":  analysis.TypeBool,
	"isempty": analysis.TypeBool,
	"isnan":   analysis.TypeBool,

	// Scalar functions
	"size":             analysis.TypeInt,
	"length":           analysis.TypeInt,
	"char_length":      analysis.TypeInt,
	"character_length": analysis.TypeInt,
	"type":             analysis.TypeString,
	"valuetype":        analysis.TypeString,
	"id":               analysis.TypeInt,
	"elementid":        analysis.TypeString,
	"labels":           analysis.SliceOf(analysis.TypeString),
	"keys":             analysis.SliceOf(analysis.TypeString),
	"randomuuid":       analysis.TypeString,
	"timestamp":        analysis.TypeInt,
	"range":            analysis.SliceOf(analysis.TypeInt),

	// Graph element functions
	"startnode": nil,
	"endnode":   nil,
	"nullif":    nil,

	// Math functions
	"abs":   analysis.TypeFloat64,
	"ceil":  analysis.TypeFloat64,
	"floor": analysis.TypeFloat64,
	"round": analysis.TypeFloat64,
	"sign":  analysis.TypeInt,
	"rand":  analysis.TypeFloat64,

	// Math functions - logarithmic
	"sqrt":  analysis.TypeFloat64,
	"log":   analysis.TypeFloat64,
	"log10": analysis.TypeFloat64,
	"exp":   analysis.TypeFloat64,
	"e":     analysis.TypeFloat64,
	"pi":    analysis.TypeFloat64,

	// Trigonometric functions
	"sin":      analysis.TypeFloat64,
	"cos":      analysis.TypeFloat64,
	"tan":      analysis.TypeFloat64,
	"cot":      analysis.TypeFloat64,
	"asin":     analysis.TypeFloat64,
	"acos":     analysis.TypeFloat64,
	"atan":     analysis.TypeFloat64,
	"atan2":    analysis.TypeFloat64,
	"degrees":  analysis.TypeFloat64,
	"radians":  analysis.TypeFloat64,
	"haversin": analysis.TypeFloat64,

	// String functions
	"left":      analysis.TypeString,
	"right":     analysis.TypeString,
	"ltrim":     analysis.TypeString,
	"rtrim":     analysis.TypeString,
	"btrim":     analysis.TypeString,
	"trim":      analysis.TypeString,
	"tolower":   analysis.TypeString,
	"toupper":   analysis.TypeString,
	"replace":   analysis.TypeString,
	"substring": analysis.TypeString,
	"reverse":   analysis.TypeString,
	"split":     analysis.SliceOf(analysis.TypeString),
	"normalize": analysis.TypeString,
	"lpad":      analysis.TypeString,
	"rpad":      analysis.TypeString,
	"repeat":    analysis.TypeString,
	"concat":    analysis.TypeString,

	// Temporal functions
	"date":          analysis.NamedType("time", "Time"),
	"datetime":      analysis.NamedType("time", "Time"),
	"localdatetime": analysis.NamedType("time", "Time"),
	"localtime":     analysis.NamedType("time", "Time"),
	"time":          analysis.NamedType("time", "Time"),
	"duration":      analysis.NamedType("time", "Duration"),

	// Temporal - current values
	"date.realtime":          analysis.NamedType("time", "Time"),
	"date.statement":         analysis.NamedType("time", "Time"),
	"date.transaction":       analysis.NamedType("time", "Time"),
	"datetime.realtime":      analysis.NamedType("time", "Time"),
	"datetime.statement":     analysis.NamedType("time", "Time"),
	"datetime.transaction":   analysis.NamedType("time", "Time"),
	"localdatetime.realtime": analysis.NamedType("time", "Time"),
	"localtime.realtime":     analysis.NamedType("time", "Time"),
	"time.realtime":          analysis.NamedType("time", "Time"),

	// Temporal - truncation
	"date.truncate":          analysis.NamedType("time", "Time"),
	"datetime.truncate":      analysis.NamedType("time", "Time"),
	"localdatetime.truncate": analysis.NamedType("time", "Time"),
	"localtime.truncate":     analysis.NamedType("time", "Time"),
	"time.truncate":          analysis.NamedType("time", "Time"),

	// Temporal - duration
	"duration.between":   analysis.NamedType("time", "Duration"),
	"duration.inmonths":  analysis.NamedType("time", "Duration"),
	"duration.indays":    analysis.NamedType("time", "Duration"),
	"duration.inseconds": analysis.NamedType("time", "Duration"),

	// Spatial functions
	"point":            analysis.NamedType("neo4j", "Point"),
	"point.withinbbox": analysis.TypeBool,
	"distance":         analysis.TypeFloat64,

	// ============================================================================
	// APOC Functions
	// ============================================================================

	// APOC - Text functions
	"apoc.text.join":                   analysis.TypeString,
	"apoc.text.replace":                analysis.TypeString,
	"apoc.text.regexgroups":            analysis.SliceOf(analysis.SliceOf(analysis.TypeString)),
	"apoc.text.split":                  analysis.SliceOf(analysis.TypeString),
	"apoc.text.capitalize":             analysis.TypeString,
	"apoc.text.capitalizall":           analysis.TypeString,
	"apoc.text.decapitalize":           analysis.TypeString,
	"apoc.text.decapitalizeall":        analysis.TypeString,
	"apoc.text.swapcase":               analysis.TypeString,
	"apoc.text.camelcase":              analysis.TypeString,
	"apoc.text.snakecase":              analysis.TypeString,
	"apoc.text.uppercamelcase":         analysis.TypeString,
	"apoc.text.lowercamelcase":         analysis.TypeString,
	"apoc.text.random":                 analysis.TypeString,
	"apoc.text.format":                 analysis.TypeString,
	"apoc.text.lpad":                   analysis.TypeString,
	"apoc.text.rpad":                   analysis.TypeString,
	"apoc.text.clean":                  analysis.TypeString,
	"apoc.text.compareclean":           analysis.TypeInt,
	"apoc.text.distance":               analysis.TypeInt,
	"apoc.text.levenshtein":            analysis.TypeFloat64,
	"apoc.text.jarowinklerdistance":    analysis.TypeFloat64,
	"apoc.text.sorensendicecoefficent": analysis.TypeFloat64,
	"apoc.text.fuzzymatch":             analysis.TypeBool,
	"apoc.text.urlencode":              analysis.TypeString,
	"apoc.text.urldecode":              analysis.TypeString,
	"apoc.text.bytes":                  analysis.SliceOf(analysis.TypeInt),
	"apoc.text.regreplace":             analysis.TypeString,
	"apoc.text.indexof":                analysis.TypeInt,

	// APOC - Date/time functions
	"apoc.date.format":                analysis.TypeString,
	"apoc.date.parse":                 analysis.TypeInt,
	"apoc.date.parseaszondeddatetime": analysis.NamedType("time", "Time"),
	"apoc.date.currenttimestamp":      analysis.TypeInt,
	"apoc.date.toiso8601":             analysis.TypeString,
	"apoc.date.fromiso8601":           analysis.TypeInt,
	"apoc.date.fields":                analysis.MapOf(analysis.TypeString, analysis.TypeInt),
	"apoc.date.field":                 analysis.TypeInt,
	"apoc.date.add":                   analysis.TypeInt,
	"apoc.date.convert":               analysis.TypeInt,
	"apoc.temporal.format":            analysis.TypeString,
	"apoc.temporal.tozonedtemporal":   analysis.NamedType("time", "Time"),

	// APOC - Collection functions
	"apoc.coll.sum":                    analysis.TypeFloat64,
	"apoc.coll.avg":                    analysis.TypeFloat64,
	"apoc.coll.min":                    nil,
	"apoc.coll.max":                    nil,
	"apoc.coll.sumlongs":               analysis.TypeInt64,
	"apoc.coll.sort":                   nil,
	"apoc.coll.sortnodes":              analysis.SliceOf(nil),
	"apoc.coll.sortmaps":               analysis.SliceOf(analysis.MapOf(analysis.TypeString, nil)),
	"apoc.coll.reverse":                nil,
	"apoc.coll.contains":               analysis.TypeBool,
	"apoc.coll.containsall":            analysis.TypeBool,
	"apoc.coll.containssorted":         analysis.TypeBool,
	"apoc.coll.containsallsorted":      analysis.TypeBool,
	"apoc.coll.union":                  nil,
	"apoc.coll.unionall":               nil,
	"apoc.coll.subtract":               nil,
	"apoc.coll.removeall":              nil,
	"apoc.coll.intersection":           nil,
	"apoc.coll.disjunction":            nil,
	"apoc.coll.shuffle":                nil,
	"apoc.coll.randomitem":             nil,
	"apoc.coll.randomitems":            nil,
	"apoc.coll.toset":                  nil,
	"apoc.coll.duplicates":             nil,
	"apoc.coll.duplicateswithcount":    analysis.SliceOf(analysis.MapOf(analysis.TypeString, nil)),
	"apoc.coll.frequencies":            analysis.MapOf(nil, analysis.TypeInt),
	"apoc.coll.frequenciesasmap":       analysis.MapOf(nil, analysis.TypeInt),
	"apoc.coll.occurrences":            analysis.TypeInt,
	"apoc.coll.flatten":                nil,
	"apoc.coll.combinations":           analysis.SliceOf(nil),
	"apoc.coll.different":              analysis.TypeBool,
	"apoc.coll.dropduplicateneighbors": nil,
	"apoc.coll.fill":                   nil,
	"apoc.coll.indexof":                analysis.TypeInt,
	"apoc.coll.insert":                 nil,
	"apoc.coll.insertall":              nil,
	"apoc.coll.isempty":                analysis.TypeBool,
	"apoc.coll.pairsmin":               analysis.SliceOf(analysis.SliceOf(nil)),
	"apoc.coll.pairs":                  analysis.SliceOf(analysis.SliceOf(nil)),
	"apoc.coll.partition":              analysis.SliceOf(analysis.SliceOf(nil)),
	"apoc.coll.remove":                 nil,
	"apoc.coll.split":                  analysis.MapOf(analysis.TypeString, analysis.SliceOf(nil)),
	"apoc.coll.ziptorows":              analysis.SliceOf(nil),

	// APOC - Map functions
	"apoc.map.flatten":          analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.fromlists":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.frompairs":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.fromnodes":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.fromvalues":       analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.merge":            analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.mergelist":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.setkey":           analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.setentry":         analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.setlists":         analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.setpairs":         analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.setvalues":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.removekey":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.removekeys":       analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.clean":            analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.groupby":          analysis.MapOf(analysis.TypeString, analysis.SliceOf(nil)),
	"apoc.map.groupbymulti":     analysis.MapOf(analysis.TypeString, analysis.SliceOf(nil)),
	"apoc.map.sortedproperties": analysis.SliceOf(analysis.SliceOf(nil)),
	"apoc.map.updatetree":       analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.values":           analysis.SliceOf(nil),
	"apoc.map.submap":           analysis.MapOf(analysis.TypeString, nil),
	"apoc.map.mget":             analysis.SliceOf(nil),
	"apoc.map.get":              nil,

	// APOC - Conversion functions
	"apoc.convert.tomap":              analysis.MapOf(analysis.TypeString, nil),
	"apoc.convert.tolist":             analysis.SliceOf(nil),
	"apoc.convert.tonode":             nil,
	"apoc.convert.torelationship":     nil,
	"apoc.convert.toset":              analysis.SliceOf(nil),
	"apoc.convert.tosortedjsonmap":    analysis.TypeString,
	"apoc.convert.tojson":             analysis.TypeString,
	"apoc.convert.fromjsonmap":        analysis.MapOf(analysis.TypeString, nil),
	"apoc.convert.fromjsonlist":       analysis.SliceOf(nil),
	"apoc.convert.getjsonproperty":    nil,
	"apoc.convert.getjsonpropertymap": analysis.MapOf(analysis.TypeString, nil),
	"apoc.convert.setjsonproperty":    nil,
	"apoc.convert.toboolean":          analysis.TypeBool,
	"apoc.convert.tofloat":            analysis.TypeFloat64,
	"apoc.convert.tointeger":          analysis.TypeInt,
	"apoc.convert.tostring":           analysis.TypeString,

	// APOC - Math functions
	"apoc.math.maxlong":   analysis.TypeInt64,
	"apoc.math.minlong":   analysis.TypeInt64,
	"apoc.math.maxdouble": analysis.TypeFloat64,
	"apoc.math.mindouble": analysis.TypeFloat64,
	"apoc.math.maxint":    analysis.TypeInt,
	"apoc.math.minint":    analysis.TypeInt,

	// APOC - Hashing functions
	"apoc.hashing.fingerprint":      analysis.TypeString,
	"apoc.hashing.fingerprintgraph": analysis.TypeString,
	"apoc.hashing.fingerprinting":   analysis.TypeString,
	"apoc.util.md5":                 analysis.TypeString,
	"apoc.util.sha1":                analysis.TypeString,
	"apoc.util.sha256":              analysis.TypeString,
	"apoc.util.sha384":              analysis.TypeString,
	"apoc.util.sha512":              analysis.TypeString,

	// APOC - Node/Relationship functions
	"apoc.node.id":                  analysis.TypeInt,
	"apoc.node.labels":              analysis.SliceOf(analysis.TypeString),
	"apoc.node.degree":              analysis.TypeInt,
	"apoc.node.degree.in":           analysis.TypeInt,
	"apoc.node.degree.out":          analysis.TypeInt,
	"apoc.node.relationship.exists": analysis.TypeBool,
	"apoc.node.relationships.exist": analysis.MapOf(analysis.TypeString, analysis.TypeBool),
	"apoc.nodes.connected":          analysis.TypeBool,
	"apoc.nodes.isdense":            analysis.TypeBool,
	"apoc.rel.id":                   analysis.TypeInt,
	"apoc.rel.type":                 analysis.TypeString,
	"apoc.rel.startnode":            nil,
	"apoc.rel.endnode":              nil,
	"apoc.any.property":             nil,
	"apoc.any.properties":           analysis.MapOf(analysis.TypeString, nil),

	// APOC - Path functions
	"apoc.path.elements": analysis.SliceOf(nil),
	"apoc.path.combine":  nil,
	"apoc.path.create":   nil,
	"apoc.path.slice":    nil,

	// APOC - Scoring functions
	"apoc.scoring.existence": analysis.TypeFloat64,
	"apoc.scoring.pareto":    analysis.TypeFloat64,

	// APOC - Bitwise functions
	"apoc.bitwise.op": analysis.TypeInt64,

	// APOC - Number functions
	"apoc.number.format":          analysis.TypeString,
	"apoc.number.parsefloat":      analysis.TypeFloat64,
	"apoc.number.parseint":        analysis.TypeInt,
	"apoc.number.exact.add":       analysis.TypeString,
	"apoc.number.exact.sub":       analysis.TypeString,
	"apoc.number.exact.mul":       analysis.TypeString,
	"apoc.number.exact.div":       analysis.TypeString,
	"apoc.number.exact.tointeger": analysis.TypeInt,
	"apoc.number.exact.tofloat":   analysis.TypeFloat64,
	"apoc.number.exact.toexact":   analysis.TypeString,

	// APOC - JSON functions
	"apoc.json.path": nil,
}

// functionsWithArgInference lists function names that need argument-based type inference.
var functionsWithArgInference = map[string]bool{
	"collect":       true,
	"head":          true,
	"last":          true,
	"tail":          true,
	"coalesce":      true,
	"min":           true,
	"max":           true,
	"properties":    true,
	"nodes":         true,
	"relationships": true,
}

// inferFunctionInvocation determines the return type of a function call.
func inferFunctionInvocation(fc *cyphergrammar.FunctionCall, qctx *queryContext) *analysis.Type {
	if fc == nil || fc.Name == nil {
		return nil
	}

	funcName := fc.Name.String()
	lowerName := strings.ToLower(funcName)

	// Check fixed return type first
	if typ, ok := cypherFunctionTypes[lowerName]; ok {
		return typ
	}

	// Check if this function needs argument-based inference
	if functionsWithArgInference[lowerName] {
		return inferFunctionWithArgs(lowerName, fc.Args, qctx)
	}

	return nil
}

// inferFunctionWithArgs handles functions whose return type depends on arguments.
func inferFunctionWithArgs(funcName string, args []*cyphergrammar.Expression, qctx *queryContext) *analysis.Type {
	getArg := func(idx int) *analysis.Type {
		if idx >= len(args) {
			return nil
		}
		return inferExpression(args[idx], qctx)
	}

	switch funcName {
	case "collect":
		// collect(x) → []typeof(x)
		elemType := getArg(0)
		return analysis.SliceOf(elemType)

	case "head", "last":
		// head(list) / last(list) → element type
		listType := getArg(0)
		if listType != nil && listType.Kind == analysis.TypeKindSlice {
			return listType.Elem
		}
		return nil

	case "tail":
		// tail(list) → same list type
		return getArg(0)

	case "coalesce":
		// coalesce(a, b, ...) → type of first argument
		return getArg(0)

	case "min", "max":
		// min/max(x) → same type as x
		return getArg(0)

	case "properties":
		// properties(node) → map[string]any
		return analysis.MapOf(analysis.TypeString, nil)

	case "nodes", "relationships":
		// nodes(path) / relationships(path) → list
		return analysis.SliceOf(nil)

	default:
		return nil
	}
}
