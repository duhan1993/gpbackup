package backup

/*
 * This file contains structs and functions related to dumping metadata on the
 * master for objects that connect to external data (external tables and external
 * protocols).
 */

import (
	"fmt"
	"strings"

	"github.com/greenplum-db/gpbackup/utils"
)

const (
	// Type of external table
	READABLE = iota
	READABLE_WEB
	WRITABLE
	WRITABLE_WEB
	// Protocol external table is using
	FILE
	GPFDIST
	GPHDFS
	HTTP
	S3
)

type ExternalTableDefinition struct {
	Oid             uint32
	Type            int
	Protocol        int
	Location        string
	ExecLocation    string
	FormatType      string
	FormatOpts      string
	Options         string
	Command         string
	RejectLimit     int
	RejectLimitType string
	ErrTable        string
	Encoding        string
	Writable        bool
	URIs            []string
}

func PrintExternalTableCreateStatement(predataFile *utils.FileWithByteCount, toc *utils.TOC, table Relation, tableDef TableDefinition) {
	start := predataFile.ByteCount
	tableTypeStrMap := map[int]string{
		READABLE:     "READABLE EXTERNAL",
		READABLE_WEB: "READABLE EXTERNAL WEB",
		WRITABLE:     "WRITABLE EXTERNAL",
		WRITABLE_WEB: "WRITABLE EXTERNAL WEB",
	}
	extTableDef := tableDef.ExtTableDef
	extTableDef.Type, extTableDef.Protocol = DetermineExternalTableCharacteristics(extTableDef)
	predataFile.MustPrintf("\n\nCREATE %s TABLE %s (\n", tableTypeStrMap[extTableDef.Type], table.ToString())
	printColumnDefinitions(predataFile, table, tableDef.ColumnDefs)
	predataFile.MustPrintf(") ")
	PrintExternalTableStatements(predataFile, table, extTableDef)
	if extTableDef.Writable {
		predataFile.MustPrintf("\n%s", tableDef.DistPolicy)
	}
	predataFile.MustPrintf(";")
	toc.AddPredataEntry(table.SchemaName, table.RelationName, "TABLE", start, predataFile.ByteCount)
}

func DetermineExternalTableCharacteristics(extTableDef ExternalTableDefinition) (int, int) {
	isWritable := extTableDef.Writable
	tableType := -1
	tableProtocol := -1
	if extTableDef.Location == "" { // EXTERNAL WEB tables may have EXECUTE instead of LOCATION
		tableProtocol = HTTP
		if isWritable {
			tableType = WRITABLE_WEB
		} else {
			tableType = READABLE_WEB
		}
	} else {
		/*
		 * All data sources must use the same protocol, so we can use Location to determine
		 * the table's protocol even though it only holds one data source URI.
		 */
		isWeb := strings.HasPrefix(extTableDef.Location, "http")
		if isWeb && isWritable {
			tableType = WRITABLE_WEB
		} else if isWeb && !isWritable {
			tableType = READABLE_WEB
		} else if !isWeb && isWritable {
			tableType = WRITABLE
		} else {
			tableType = READABLE
		}
		prefix := extTableDef.Location[0:strings.Index(extTableDef.Location, "://")]
		switch prefix {
		case "file":
			tableProtocol = FILE
		case "gpfdist":
			tableProtocol = GPFDIST
		case "gpfdists":
			tableProtocol = GPFDIST
		case "gphdfs":
			tableProtocol = GPHDFS
		case "http":
			tableProtocol = HTTP
		case "https":
			tableProtocol = HTTP
		case "s3":
			tableProtocol = S3
		}
	}
	return tableType, tableProtocol
}

func PrintExternalTableStatements(predataFile *utils.FileWithByteCount, table Relation, extTableDef ExternalTableDefinition) {
	if extTableDef.Type != WRITABLE_WEB {
		if len(extTableDef.URIs) > 0 {
			predataFile.MustPrintf("LOCATION (\n\t'%s'\n)", strings.Join(extTableDef.URIs, "',\n\t'"))
		}
	}
	if extTableDef.Type == READABLE || (extTableDef.Type == WRITABLE_WEB && extTableDef.Protocol == S3) {
		if extTableDef.ExecLocation == "MASTER_ONLY" {
			predataFile.MustPrintf(" ON MASTER")
		}
	}
	if extTableDef.Type == READABLE_WEB || extTableDef.Type == WRITABLE_WEB {
		if extTableDef.Command != "" {
			predataFile.MustPrintf("EXECUTE '%s'", extTableDef.Command)
			execType := strings.Split(extTableDef.ExecLocation, ":")
			switch execType[0] {
			case "ALL_SEGMENTS": // Default case, don't print anything else
			case "HOST":
				predataFile.MustPrintf(" ON HOST '%s'", execType[1])
			case "MASTER_ONLY":
				predataFile.MustPrintf(" ON MASTER")
			case "PER_HOST":
				predataFile.MustPrintf(" ON HOST")
			case "SEGMENT_ID":
				predataFile.MustPrintf(" ON SEGMENT %s", execType[1])
			case "TOTAL_SEGS":
				predataFile.MustPrintf(" ON %s", execType[1])
			}
		}
	}
	predataFile.MustPrintln()
	formatType := ""
	switch extTableDef.FormatType {
	case "a":
		formatType = "avro"
	case "b":
		formatType = "custom"
	case "c":
		formatType = "csv"
	case "p":
		formatType = "parquet"
	case "t":
		formatType = "text"
	}
	/*
	 * The options for the custom formatter is stored as "formatter 'function_name'",
	 * but FORMAT requires "formatter='function_name'".
	 */
	extTableDef.FormatOpts = strings.Replace(extTableDef.FormatOpts, "formatter ", "formatter=", 1)
	predataFile.MustPrintf("FORMAT '%s'", formatType)
	if extTableDef.FormatOpts != "" {
		predataFile.MustPrintf(" (%s)", strings.TrimSpace(extTableDef.FormatOpts))
	}
	predataFile.MustPrintln()
	if extTableDef.Options != "" {
		predataFile.MustPrintf("OPTIONS (\n\t%s\n)\n", extTableDef.Options)
	}
	predataFile.MustPrintf("ENCODING '%s'", extTableDef.Encoding)
	if extTableDef.Type == READABLE || extTableDef.Type == READABLE_WEB {
		/*
		 * In GPDB 5 and later, LOG ERRORS INTO [table] has been replaced by LOG ERRORS,
		 * but it still uses the same catalog entries to store that information.  If the
		 * value of pg_exttable.fmterrtbl matches the table's own name, LOG ERRORS is set.
		 */
		if extTableDef.ErrTable == table.RelationName {
			predataFile.MustPrintf("\nLOG ERRORS")
		}
		if extTableDef.RejectLimit != 0 {
			predataFile.MustPrintf("\nSEGMENT REJECT LIMIT %d ", extTableDef.RejectLimit)
			switch extTableDef.RejectLimitType {
			case "r":
				predataFile.MustPrintf("ROWS")
			case "p":
				predataFile.MustPrintf("PERCENT")
			}
		}
	}
}

func PrintCreateExternalProtocolStatements(predataFile *utils.FileWithByteCount, toc *utils.TOC, protocols []ExternalProtocol, funcInfoMap map[uint32]FunctionInfo, protoMetadata MetadataMap) {
	for _, protocol := range protocols {
		start := predataFile.ByteCount
		hasUserDefinedFunc := false
		if function, ok := funcInfoMap[protocol.WriteFunction]; ok && !function.IsInternal {
			hasUserDefinedFunc = true
		}
		if function, ok := funcInfoMap[protocol.ReadFunction]; ok && !function.IsInternal {
			hasUserDefinedFunc = true
		}
		if function, ok := funcInfoMap[protocol.Validator]; ok && !function.IsInternal {
			hasUserDefinedFunc = true
		}

		if !hasUserDefinedFunc {
			continue
		}

		protocolFunctions := []string{}
		if protocol.ReadFunction != 0 {
			protocolFunctions = append(protocolFunctions, fmt.Sprintf("readfunc = %s", funcInfoMap[protocol.ReadFunction].QualifiedName))
		}
		if protocol.WriteFunction != 0 {
			protocolFunctions = append(protocolFunctions, fmt.Sprintf("writefunc = %s", funcInfoMap[protocol.WriteFunction].QualifiedName))
		}
		if protocol.Validator != 0 {
			protocolFunctions = append(protocolFunctions, fmt.Sprintf("validatorfunc = %s", funcInfoMap[protocol.Validator].QualifiedName))
		}

		predataFile.MustPrintf("\n\nCREATE ")
		if protocol.Trusted {
			predataFile.MustPrintf("TRUSTED ")
		}
		protoFQN := utils.QuoteIdent(protocol.Name)
		predataFile.MustPrintf("PROTOCOL %s (%s);\n", protoFQN, strings.Join(protocolFunctions, ", "))
		PrintObjectMetadata(predataFile, protoMetadata[protocol.Oid], protoFQN, "PROTOCOL")
		toc.AddPredataEntry("", protocol.Name, "PROTOCOL", start, predataFile.ByteCount)
	}
}
