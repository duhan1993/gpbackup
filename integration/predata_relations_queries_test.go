package integration

import (
	"sort"

	"github.com/greenplum-db/gp-common-go-libs/structmatcher"
	"github.com/greenplum-db/gp-common-go-libs/testhelper"
	"github.com/greenplum-db/gpbackup/backup"
	"github.com/greenplum-db/gpbackup/testutils"
	"github.com/greenplum-db/gpbackup/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"math"
)

var _ = Describe("backup integration tests", func() {
	Describe("GetAllUserTables", func() {
		It("returns user table information for basic heap tables", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.foo")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema CASCADE")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE testschema.testtable(t text)")

			tables := backup.GetAllUserTables(connectionPool)

			tableFoo := backup.Relation{Schema: "public", Name: "foo"}

			tableTestTable := backup.Relation{Schema: "testschema", Name: "testtable"}

			Expect(tables).To(HaveLen(2))
			structmatcher.ExpectStructsToMatchExcluding(&tableFoo, &tables[0], "SchemaOid", "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&tableTestTable, &tables[1], "SchemaOid", "Oid")
		})
		Context("Retrieving external partitions", func() {
			It("returns parent and external leaf partition table if the filter includes a leaf table and leaf-partition-data is set", func() {
				backupCmdFlags.Set(utils.LEAF_PARTITION_DATA, "true")
				backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.partition_table_1_prt_boys")
				testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.partition_table (id int, gender char(1))
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`)
				testhelper.AssertQueryRuns(connectionPool, `CREATE EXTERNAL WEB TABLE public.partition_table_ext_part_ (like public.partition_table_1_prt_girls)
EXECUTE 'echo -e "2\n1"' on host
FORMAT 'csv';`)
				testhelper.AssertQueryRuns(connectionPool, `ALTER TABLE public.partition_table EXCHANGE PARTITION girls WITH TABLE public.partition_table_ext_part_ WITHOUT VALIDATION;`)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table")
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table_ext_part_")

				tables := backup.GetAllUserTables(connectionPool)

				expectedTableNames := []string{"public.partition_table", "public.partition_table_1_prt_boys", "public.partition_table_1_prt_girls"}
				tableNames := make([]string, 0)
				for _, table := range tables {
					tableNames = append(tableNames, table.FQN())
				}
				sort.Strings(tableNames)

				Expect(tables).To(HaveLen(3))
				Expect(tableNames).To(Equal(expectedTableNames))
			})
			It("returns external partition tables for an included parent table if the filter includes a parent partition table", func() {
				backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.partition_table1,public.partition_table2_1_prt_other")
				testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.partition_table1 (id int, gender char(1))
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`)
				testhelper.AssertQueryRuns(connectionPool, `CREATE EXTERNAL WEB TABLE public.partition_table1_ext_part_ (like public.partition_table1_1_prt_boys)
EXECUTE 'echo -e "2\n1"' on host
FORMAT 'csv';`)
				testhelper.AssertQueryRuns(connectionPool, `ALTER TABLE public.partition_table1 EXCHANGE PARTITION boys WITH TABLE public.partition_table1_ext_part_ WITHOUT VALIDATION;`)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table1")
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table1_ext_part_")
				testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.partition_table2 (id int, gender char(1))
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`)
				testhelper.AssertQueryRuns(connectionPool, `CREATE EXTERNAL WEB TABLE public.partition_table2_ext_part_ (like public.partition_table2_1_prt_girls)
EXECUTE 'echo -e "2\n1"' on host
FORMAT 'csv';`)
				testhelper.AssertQueryRuns(connectionPool, `ALTER TABLE public.partition_table2 EXCHANGE PARTITION girls WITH TABLE public.partition_table2_ext_part_ WITHOUT VALIDATION;`)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table2")
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table2_ext_part_")
				testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.partition_table3 (id int, gender char(1))
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`)
				testhelper.AssertQueryRuns(connectionPool, `CREATE EXTERNAL WEB TABLE public.partition_table3_ext_part_ (like public.partition_table3_1_prt_girls)
EXECUTE 'echo -e "2\n1"' on host
FORMAT 'csv';`)
				testhelper.AssertQueryRuns(connectionPool, `ALTER TABLE public.partition_table3 EXCHANGE PARTITION girls WITH TABLE public.partition_table3_ext_part_ WITHOUT VALIDATION;`)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table3")
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table3_ext_part_")

				tables := backup.GetAllUserTables(connectionPool)

				expectedTableNames := []string{"public.partition_table1", "public.partition_table1_1_prt_boys", "public.partition_table2", "public.partition_table2_1_prt_girls", "public.partition_table2_1_prt_other"}
				tableNames := make([]string, 0)
				for _, table := range tables {
					tableNames = append(tableNames, table.FQN())
				}
				sort.Strings(tableNames)

				Expect(tables).To(HaveLen(5))
				Expect(tableNames).To(Equal(expectedTableNames))
			})
		})
		Context("leaf-partition-data flag", func() {
			It("returns only parent partition tables if the leaf-partition-data flag is not set and there are no include tables", func() {
				createStmt := `CREATE TABLE public.rank (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`
				testhelper.AssertQueryRuns(connectionPool, createStmt)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.rank")

				tables := backup.GetAllUserTables(connectionPool)

				tableRank := backup.Relation{Schema: "public", Name: "rank"}

				Expect(tables).To(HaveLen(1))
				structmatcher.ExpectStructsToMatchExcluding(&tableRank, &tables[0], "SchemaOid", "Oid")
			})
			It("returns both parent and leaf partition tables if the leaf-partition-data flag is set and there are no include tables", func() {
				backupCmdFlags.Set(utils.LEAF_PARTITION_DATA, "true")
				createStmt := `CREATE TABLE public.rank (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`
				testhelper.AssertQueryRuns(connectionPool, createStmt)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.rank")

				tables := backup.GetAllUserTables(connectionPool)

				expectedTableNames := []string{"public.rank", "public.rank_1_prt_boys", "public.rank_1_prt_girls", "public.rank_1_prt_other"}
				tableNames := make([]string, 0)
				for _, table := range tables {
					tableNames = append(tableNames, table.FQN())
				}
				sort.Strings(tableNames)

				Expect(tables).To(HaveLen(4))
				Expect(tableNames).To(Equal(expectedTableNames))
			})
			It("returns parent and included child partition table if the filter includes a leaf table; with and without leaf-partition-data", func() {
				backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.rank_1_prt_girls")
				createStmt := `CREATE TABLE public.rank (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`
				testhelper.AssertQueryRuns(connectionPool, createStmt)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.rank")

				expectedTableNames := []string{"public.rank", "public.rank_1_prt_girls"}

				tables := backup.GetAllUserTables(connectionPool)
				tableNames := make([]string, 0)
				for _, table := range tables {
					tableNames = append(tableNames, table.FQN())
				}
				sort.Strings(tableNames)

				Expect(tables).To(HaveLen(2))
				Expect(tableNames).To(Equal(expectedTableNames))

				backupCmdFlags.Set(utils.LEAF_PARTITION_DATA, "true")
				tables = backup.GetAllUserTables(connectionPool)
				tableNames = make([]string, 0)
				for _, table := range tables {
					tableNames = append(tableNames, table.FQN())
				}
				sort.Strings(tableNames)

				Expect(tables).To(HaveLen(2))
				Expect(tableNames).To(Equal(expectedTableNames))
			})
			It("returns child partition tables for an included parent table if the leaf-partition-data flag is set and the filter includes a parent partition table", func() {
				backupCmdFlags.Set(utils.LEAF_PARTITION_DATA, "true")
				backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.rank")
				createStmt := `CREATE TABLE public.rank (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );`
				testhelper.AssertQueryRuns(connectionPool, createStmt)
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.rank")
				testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.test_table(i int)")
				defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.test_table")

				tables := backup.GetAllUserTables(connectionPool)

				expectedTableNames := []string{"public.rank", "public.rank_1_prt_boys", "public.rank_1_prt_girls", "public.rank_1_prt_other"}
				tableNames := make([]string, 0)
				for _, table := range tables {
					tableNames = append(tableNames, table.FQN())
				}
				sort.Strings(tableNames)

				Expect(tables).To(HaveLen(4))
				Expect(tableNames).To(Equal(expectedTableNames))
			})
		})
		It("returns user table information for table in specific schema", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.foo")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE testschema.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE testschema.foo")

			backupCmdFlags.Set(utils.INCLUDE_SCHEMA, "testschema")
			tables := backup.GetAllUserTables(connectionPool)

			tableFoo := backup.Relation{Schema: "testschema", Name: "foo"}

			Expect(tables).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&tableFoo, &tables[0], "SchemaOid", "Oid")
		})
		It("returns user table information for tables in includeTables", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.foo")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE testschema.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE testschema.foo")

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "testschema.foo")
			tables := backup.GetAllUserTables(connectionPool)

			tableFoo := backup.Relation{Schema: "testschema", Name: "foo"}

			Expect(tables).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&tableFoo, &tables[0], "SchemaOid", "Oid")
		})
		It("returns user table information for tables not in excludeTables", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.foo")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE testschema.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE testschema.foo")

			backupCmdFlags.Set(utils.EXCLUDE_RELATION, "testschema.foo")
			tables := backup.GetAllUserTables(connectionPool)

			tableFoo := backup.Relation{Schema: "public", Name: "foo"}

			Expect(tables).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&tableFoo, &tables[0], "SchemaOid", "Oid")
		})
		It("returns user table information for tables in includeSchema but not in excludeTables", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.foo")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE testschema.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE testschema.foo")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE testschema.bar(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE testschema.bar")

			backupCmdFlags.Set(utils.INCLUDE_SCHEMA, "testschema")
			backupCmdFlags.Set(utils.EXCLUDE_RELATION, "testschema.foo")
			tables := backup.GetAllUserTables(connectionPool)

			tableFoo := backup.Relation{Schema: "testschema", Name: "bar"}
			Expect(tables).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&tableFoo, &tables[0], "SchemaOid", "Oid")
		})
		It("returns user table information for tables even with an non existant excludeTable", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.foo(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.foo")

			backupCmdFlags.Set(utils.EXCLUDE_RELATION, "testschema.nonexistant")
			tables := backup.GetAllUserTables(connectionPool)

			tableFoo := backup.Relation{Schema: "public", Name: "foo"}

			Expect(tables).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&tableFoo, &tables[0], "SchemaOid", "Oid")
		})
	})
	Describe("GetPartitionTableMap", func() {
		It("correctly maps oids to parent or leaf table types", func() {
			createStmt := `CREATE TABLE public.summer_sales (id int, year int, month int)
DISTRIBUTED BY (id)
PARTITION BY RANGE (year)
    SUBPARTITION BY RANGE (month)
       SUBPARTITION TEMPLATE (
        START (6) END (8) EVERY (1),
        DEFAULT SUBPARTITION other_months )
( START (2015) END (2017) EVERY (1),
  DEFAULT PARTITION outlying_years );
`
			testhelper.AssertQueryRuns(connectionPool, createStmt)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.summer_sales")

			parent := testutils.OidFromObjectName(connectionPool, "public", "summer_sales", backup.TYPE_RELATION)
			intermediate1 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_outlying_years", backup.TYPE_RELATION)
			leaf11 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_outlying_years_2_prt_2", backup.TYPE_RELATION)
			leaf12 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_outlying_years_2_prt_3", backup.TYPE_RELATION)
			leaf13 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_outlying_years_2_prt_other_months", backup.TYPE_RELATION)
			intermediate2 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_2", backup.TYPE_RELATION)
			leaf21 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_2_2_prt_2", backup.TYPE_RELATION)
			leaf22 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_2_2_prt_3", backup.TYPE_RELATION)
			leaf23 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_2_2_prt_other_months", backup.TYPE_RELATION)
			intermediate3 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_3", backup.TYPE_RELATION)
			leaf31 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_3_2_prt_2", backup.TYPE_RELATION)
			leaf32 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_3_2_prt_3", backup.TYPE_RELATION)
			leaf33 := testutils.OidFromObjectName(connectionPool, "public", "summer_sales_1_prt_3_2_prt_other_months", backup.TYPE_RELATION)
			partTableMap := backup.GetPartitionTableMap(connectionPool)

			Expect(partTableMap).To(HaveLen(13))
			structmatcher.ExpectStructsToMatch(partTableMap[parent], &backup.PartitionLevelInfo{Oid: parent, Level: "p", RootName: ""})
			structmatcher.ExpectStructsToMatch(partTableMap[intermediate1], &backup.PartitionLevelInfo{Oid: intermediate1, Level: "i", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[intermediate2], &backup.PartitionLevelInfo{Oid: intermediate2, Level: "i", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[intermediate3], &backup.PartitionLevelInfo{Oid: intermediate3, Level: "i", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf11], &backup.PartitionLevelInfo{Oid: leaf11, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf12], &backup.PartitionLevelInfo{Oid: leaf12, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf13], &backup.PartitionLevelInfo{Oid: leaf13, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf21], &backup.PartitionLevelInfo{Oid: leaf21, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf22], &backup.PartitionLevelInfo{Oid: leaf22, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf23], &backup.PartitionLevelInfo{Oid: leaf23, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf31], &backup.PartitionLevelInfo{Oid: leaf31, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf32], &backup.PartitionLevelInfo{Oid: leaf32, Level: "l", RootName: "summer_sales"})
			structmatcher.ExpectStructsToMatch(partTableMap[leaf33], &backup.PartitionLevelInfo{Oid: leaf33, Level: "l", RootName: "summer_sales"})
		})
	})
	Describe("GetColumnDefinitions", func() {
		emptyColumnACL := []backup.ACL{}
		It("returns table attribute information for a heap table", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.atttable(a float, b text, c text NOT NULL, d int DEFAULT(5), e text)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.atttable")
			testhelper.AssertQueryRuns(connectionPool, "COMMENT ON COLUMN public.atttable.a IS 'att comment'")
			testhelper.AssertQueryRuns(connectionPool, "ALTER TABLE public.atttable DROP COLUMN b")
			testhelper.AssertQueryRuns(connectionPool, "ALTER TABLE public.atttable ALTER COLUMN e SET STORAGE PLAIN")
			oid := testutils.OidFromObjectName(connectionPool, "public", "atttable", backup.TYPE_RELATION)
			privileges := backup.GetPrivilegesForColumns(connectionPool)
			tableAtts := backup.GetColumnDefinitions(connectionPool, privileges)[oid]

			columnA := backup.ColumnDefinition{Oid: 0, Num: 1, Name: "a", NotNull: false, HasDefault: false, Type: "double precision", Encoding: "", StatTarget: -1, StorageType: "", DefaultVal: "", Comment: "att comment", ACL: emptyColumnACL}
			columnC := backup.ColumnDefinition{Oid: 0, Num: 3, Name: "c", NotNull: true, HasDefault: false, Type: "text", Encoding: "", StatTarget: -1, StorageType: "", DefaultVal: "", Comment: "", ACL: emptyColumnACL}
			columnD := backup.ColumnDefinition{Oid: 0, Num: 4, Name: "d", NotNull: false, HasDefault: true, Type: "integer", Encoding: "", StatTarget: -1, StorageType: "", DefaultVal: "5", Comment: "", ACL: emptyColumnACL}
			columnE := backup.ColumnDefinition{Oid: 0, Num: 5, Name: "e", NotNull: false, HasDefault: false, Type: "text", Encoding: "", StatTarget: -1, StorageType: "PLAIN", DefaultVal: "", Comment: "", ACL: emptyColumnACL}

			Expect(tableAtts).To(HaveLen(4))

			structmatcher.ExpectStructsToMatchExcluding(&columnA, &tableAtts[0], "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&columnC, &tableAtts[1], "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&columnD, &tableAtts[2], "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&columnE, &tableAtts[3], "Oid")
		})
		It("returns table attributes including encoding for a column oriented table", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.co_atttable(a float, b text ENCODING(blocksize=65536)) WITH (appendonly=true, orientation=column)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.co_atttable")
			oid := testutils.OidFromObjectName(connectionPool, "public", "co_atttable", backup.TYPE_RELATION)
			privileges := backup.GetPrivilegesForColumns(connectionPool)
			tableAtts := backup.GetColumnDefinitions(connectionPool, privileges)[oid]

			columnA := backup.ColumnDefinition{Oid: 0, Num: 1, Name: "a", NotNull: false, HasDefault: false, Type: "double precision", Encoding: "compresstype=none,blocksize=32768,compresslevel=0", StatTarget: -1, StorageType: "", DefaultVal: "", Comment: "", ACL: emptyColumnACL}
			columnB := backup.ColumnDefinition{Oid: 0, Num: 2, Name: "b", NotNull: false, HasDefault: false, Type: "text", Encoding: "blocksize=65536,compresstype=none,compresslevel=0", StatTarget: -1, StorageType: "", DefaultVal: "", Comment: "", ACL: emptyColumnACL}

			Expect(tableAtts).To(HaveLen(2))

			structmatcher.ExpectStructsToMatchExcluding(&columnA, &tableAtts[0], "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&columnB, &tableAtts[1], "Oid")
		})
		It("returns an empty attribute array for a table with no columns", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.nocol_atttable()")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.nocol_atttable")
			oid := testutils.OidFromObjectName(connectionPool, "public", "nocol_atttable", backup.TYPE_RELATION)

			privileges := backup.GetPrivilegesForColumns(connectionPool)
			tableAtts := backup.GetColumnDefinitions(connectionPool, privileges)[oid]

			Expect(tableAtts).To(BeEmpty())
		})
		It("returns table attributes with options only applicable to master", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE COLLATION public.some_coll (lc_collate = 'POSIX', lc_ctype = 'POSIX')")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP COLLATION public.some_coll")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.atttable(i character(8) COLLATE public.some_coll)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.atttable")
			testhelper.AssertQueryRuns(connectionPool, "ALTER TABLE ONLY public.atttable ALTER COLUMN i SET (n_distinct=1);")
			oid := testutils.OidFromObjectName(connectionPool, "public", "atttable", backup.TYPE_RELATION)
			privileges := backup.GetPrivilegesForColumns(connectionPool)
			tableAtts := backup.GetColumnDefinitions(connectionPool, privileges)[oid]

			columnA := backup.ColumnDefinition{Oid: 0, Num: 1, Name: "i", NotNull: false, HasDefault: false, Type: "character(8)", Encoding: "", StatTarget: -1, StorageType: "", DefaultVal: "", Comment: "", ACL: emptyColumnACL, Options: "n_distinct=1", Collation: "public.some_coll"}

			Expect(tableAtts).To(HaveLen(1))

			structmatcher.ExpectStructsToMatchExcluding(&columnA, &tableAtts[0], "Oid")
		})
		It("returns table attributes with foriegn data options", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE FOREIGN DATA WRAPPER dummy;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP FOREIGN DATA WRAPPER dummy")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SERVER sc FOREIGN DATA WRAPPER dummy;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SERVER sc")
			testhelper.AssertQueryRuns(connectionPool, `CREATE FOREIGN TABLE public.ft1 (
	c1 integer OPTIONS (param1 'val1', param2 'val2') NOT NULL
) SERVER sc ;`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP FOREIGN TABLE public.ft1")

			privileges := backup.GetPrivilegesForColumns(connectionPool)
			oid := testutils.OidFromObjectName(connectionPool, "public", "ft1", backup.TYPE_RELATION)
			tableAtts := backup.GetColumnDefinitions(connectionPool, privileges)[oid]

			Expect(tableAtts).To(HaveLen(1))
			column1 := backup.ColumnDefinition{Oid: 0, Num: 1, Name: "c1", NotNull: true, HasDefault: false, Type: "integer", StatTarget: -1, ACL: emptyColumnACL, FdwOptions: "param1 'val1', param2 'val2'"}
			structmatcher.ExpectStructsToMatchExcluding(column1, &tableAtts[0], "Oid")
		})
	})
	Describe("GetPrivilegesForColumns", func() {
		It("Default column", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.default_privileges(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.default_privileges")

			metadataMap := backup.GetPrivilegesForColumns(connectionPool)

			oid := testutils.OidFromObjectName(connectionPool, "public", "default_privileges", backup.TYPE_RELATION)
			expectedACL := []backup.ACL{}
			Expect(metadataMap).To(HaveLen(1))
			Expect(metadataMap[oid]).To(HaveLen(1))
			Expect(metadataMap[oid]["i"]).To(Equal(expectedACL))
		})
		It("Column with granted privileges", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.granted_privileges(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.granted_privileges")
			testhelper.AssertQueryRuns(connectionPool, "GRANT SELECT (i) ON TABLE public.granted_privileges TO testrole")

			metadataMap := backup.GetPrivilegesForColumns(connectionPool)

			oid := testutils.OidFromObjectName(connectionPool, "public", "granted_privileges", backup.TYPE_RELATION)
			expectedACL := []backup.ACL{{Grantee: "testrole", Select: true}}
			Expect(metadataMap).To(HaveLen(1))
			Expect(metadataMap[oid]).To(HaveLen(1))
			Expect(metadataMap[oid]["i"]).To(Equal(expectedACL))
		})
	})
	Describe("GetDistributionPolicies", func() {
		It("returns distribution policy info for a table DISTRIBUTED RANDOMLY", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.dist_random(a int, b text) DISTRIBUTED RANDOMLY")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.dist_random")
			oid := testutils.OidFromObjectName(connectionPool, "public", "dist_random", backup.TYPE_RELATION)

			distPolicies := backup.GetDistributionPolicies(connectionPool)[oid]

			Expect(distPolicies).To(Equal("DISTRIBUTED RANDOMLY"))
		})
		It("returns distribution policy info for a table DISTRIBUTED BY one column", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.dist_one(a int, b text) DISTRIBUTED BY (a)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.dist_one")
			oid := testutils.OidFromObjectName(connectionPool, "public", "dist_one", backup.TYPE_RELATION)

			distPolicies := backup.GetDistributionPolicies(connectionPool)[oid]

			Expect(distPolicies).To(Equal("DISTRIBUTED BY (a)"))
		})
		It("returns distribution policy info for a table DISTRIBUTED BY two columns", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.dist_two(a int, b text) DISTRIBUTED BY (a, b)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.dist_two")
			oid := testutils.OidFromObjectName(connectionPool, "public", "dist_two", backup.TYPE_RELATION)

			distPolicies := backup.GetDistributionPolicies(connectionPool)[oid]

			Expect(distPolicies).To(Equal("DISTRIBUTED BY (a, b)"))
		})
		It("returns distribution policy info for a table DISTRIBUTED BY column name as keyword", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.dist_one(a int, "group" text) DISTRIBUTED BY ("group")`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.dist_one")
			oid := testutils.OidFromObjectName(connectionPool, "public", "dist_one", backup.TYPE_RELATION)

			distPolicies := backup.GetDistributionPolicies(connectionPool)[oid]

			Expect(distPolicies).To(Equal(`DISTRIBUTED BY ("group")`))
		})
		It("returns distribution policy info for a table DISTRIBUTED BY multiple columns in correct order", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.dist_one (id int, memo varchar(20), dt date, col1 varchar(20)) DISTRIBUTED BY (memo, dt, id, col1); `)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.dist_one")
			oid := testutils.OidFromObjectName(connectionPool, "public", "dist_one", backup.TYPE_RELATION)

			distPolicies := backup.GetDistributionPolicies(connectionPool)[oid]

			Expect(distPolicies).To(Equal(`DISTRIBUTED BY (memo, dt, id, col1)`))
		})
		It("returns distribution policy info for a table DISTRIBUTED REPLICATED", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.dist_one(a int, "group" text) DISTRIBUTED REPLICATED`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.dist_one")
			oid := testutils.OidFromObjectName(connectionPool, "public", "dist_one", backup.TYPE_RELATION)

			distPolicies := backup.GetDistributionPolicies(connectionPool)[oid]

			Expect(distPolicies).To(Equal(`DISTRIBUTED REPLICATED`))
		})
	})
	Describe("GetPartitionDefinitions", func() {
		It("returns empty string when no partition exists", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.simple_table(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.simple_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "simple_table", backup.TYPE_RELATION)

			result := backup.GetPartitionDefinitions(connectionPool)[oid]

			Expect(result).To(Equal(""))
		})
		It("returns a value for a partition definition", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );
			`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "part_table", backup.TYPE_RELATION)

			result := backup.GetPartitionDefinitions(connectionPool)[oid]

			// The spacing is very specific here and is output from the postgres function
			expectedResult := `PARTITION BY LIST(gender) 
          (
          PARTITION girls VALUES('F') WITH (tablename='part_table_1_prt_girls', appendonly=false ), 
          PARTITION boys VALUES('M') WITH (tablename='part_table_1_prt_boys', appendonly=false ), 
          DEFAULT PARTITION other  WITH (tablename='part_table_1_prt_other', appendonly=false )
          )`
			Expect(result).To(Equal(expectedResult))
		})
		It("returns a value for a partition definition for a specific table", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );
			`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table")
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table2 (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );
			`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table2")
			oid := testutils.OidFromObjectName(connectionPool, "public", "part_table", backup.TYPE_RELATION)

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.part_table")

			results := backup.GetPartitionDefinitions(connectionPool)
			Expect(results).To(HaveLen(1))
			result := results[oid]

			// The spacing is very specific here and is output from the postgres function
			expectedResult := `PARTITION BY LIST(gender) 
          (
          PARTITION girls VALUES('F') WITH (tablename='part_table_1_prt_girls', appendonly=false ), 
          PARTITION boys VALUES('M') WITH (tablename='part_table_1_prt_boys', appendonly=false ), 
          DEFAULT PARTITION other  WITH (tablename='part_table_1_prt_other', appendonly=false )
          )`
			Expect(result).To(Equal(expectedResult))
		})
		It("returns a value for a partition definition in a specific schema", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );
			`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema CASCADE")
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE testschema.part_table (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );
			`)
			oid := testutils.OidFromObjectName(connectionPool, "testschema", "part_table", backup.TYPE_RELATION)

			backupCmdFlags.Set(utils.INCLUDE_SCHEMA, "testschema")

			results := backup.GetPartitionDefinitions(connectionPool)
			Expect(results).To(HaveLen(1))
			result := results[oid]

			// The spacing is very specific here and is output from the postgres function
			expectedResult := `PARTITION BY LIST(gender) 
          (
          PARTITION girls VALUES('F') WITH (tablename='part_table_1_prt_girls', appendonly=false ), 
          PARTITION boys VALUES('M') WITH (tablename='part_table_1_prt_boys', appendonly=false ), 
          DEFAULT PARTITION other  WITH (tablename='part_table_1_prt_other', appendonly=false )
          )`
			Expect(result).To(Equal(expectedResult))
		})
	})
	Describe("GetPartitionTemplates", func() {
		It("returns empty string when no partition definition template exists", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.simple_table(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.simple_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "simple_table", backup.TYPE_RELATION)

			result := backup.GetPartitionTemplates(connectionPool)[oid]

			Expect(result).To(Equal(""))
		})
		It("returns a value for a subpartition template", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table (trans_id int, date date, amount decimal(9,2), region text)
  DISTRIBUTED BY (trans_id)
  PARTITION BY RANGE (date)
  SUBPARTITION BY LIST (region)
  SUBPARTITION TEMPLATE
    ( SUBPARTITION usa VALUES ('usa'),
      SUBPARTITION asia VALUES ('asia'),
      SUBPARTITION europe VALUES ('europe'),
      DEFAULT SUBPARTITION other_regions )
  ( START (date '2014-01-01') INCLUSIVE
    END (date '2014-04-01') EXCLUSIVE
    EVERY (INTERVAL '1 month') ) `)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "part_table", backup.TYPE_RELATION)

			result := backup.GetPartitionTemplates(connectionPool)[oid]

			/*
			 * The spacing is very specific here and is output from the postgres function
			 * The only difference between the below statements is spacing
			 */
			expectedResult := ""
			if connectionPool.Version.Before("6") {
				expectedResult = `ALTER TABLE public.part_table 
SET SUBPARTITION TEMPLATE  
          (
          SUBPARTITION usa VALUES('usa') WITH (tablename='part_table'), 
          SUBPARTITION asia VALUES('asia') WITH (tablename='part_table'), 
          SUBPARTITION europe VALUES('europe') WITH (tablename='part_table'), 
          DEFAULT SUBPARTITION other_regions  WITH (tablename='part_table')
          )
`
			} else {
				expectedResult = `ALTER TABLE public.part_table 
SET SUBPARTITION TEMPLATE 
          (
          SUBPARTITION usa VALUES('usa') WITH (tablename='part_table'), 
          SUBPARTITION asia VALUES('asia') WITH (tablename='part_table'), 
          SUBPARTITION europe VALUES('europe') WITH (tablename='part_table'), 
          DEFAULT SUBPARTITION other_regions  WITH (tablename='part_table')
          )
`
			}

			Expect(result).To(Equal(expectedResult))
		})
		It("returns a value for a subpartition template for a specific table", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table (trans_id int, date date, amount decimal(9,2), region text)
  DISTRIBUTED BY (trans_id)
  PARTITION BY RANGE (date)
  SUBPARTITION BY LIST (region)
  SUBPARTITION TEMPLATE
    ( SUBPARTITION usa VALUES ('usa'),
      SUBPARTITION asia VALUES ('asia'),
      SUBPARTITION europe VALUES ('europe'),
      DEFAULT SUBPARTITION other_regions )
  ( START (date '2014-01-01') INCLUSIVE
    END (date '2014-04-01') EXCLUSIVE
    EVERY (INTERVAL '1 month') ) `)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table")
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table2 (trans_id int, date date, amount decimal(9,2), region text)
  DISTRIBUTED BY (trans_id)
  PARTITION BY RANGE (date)
  SUBPARTITION BY LIST (region)
  SUBPARTITION TEMPLATE
    ( SUBPARTITION usa VALUES ('usa'),
      SUBPARTITION asia VALUES ('asia'),
      SUBPARTITION europe VALUES ('europe'),
      DEFAULT SUBPARTITION other_regions )
  ( START (date '2014-01-01') INCLUSIVE
    END (date '2014-04-01') EXCLUSIVE
    EVERY (INTERVAL '1 month') ) `)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table2")
			oid := testutils.OidFromObjectName(connectionPool, "public", "part_table", backup.TYPE_RELATION)

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.part_table")

			results := backup.GetPartitionTemplates(connectionPool)
			Expect(results).To(HaveLen(1))
			result := results[oid]

			/*
			 * The spacing is very specific here and is output from the postgres function
			 * The only difference between the below statements is spacing
			 */
			expectedResult := ""
			if connectionPool.Version.Before("6") {
				expectedResult = `ALTER TABLE public.part_table 
SET SUBPARTITION TEMPLATE  
          (
          SUBPARTITION usa VALUES('usa') WITH (tablename='part_table'), 
          SUBPARTITION asia VALUES('asia') WITH (tablename='part_table'), 
          SUBPARTITION europe VALUES('europe') WITH (tablename='part_table'), 
          DEFAULT SUBPARTITION other_regions  WITH (tablename='part_table')
          )
`
			} else {
				expectedResult = `ALTER TABLE public.part_table 
SET SUBPARTITION TEMPLATE 
          (
          SUBPARTITION usa VALUES('usa') WITH (tablename='part_table'), 
          SUBPARTITION asia VALUES('asia') WITH (tablename='part_table'), 
          SUBPARTITION europe VALUES('europe') WITH (tablename='part_table'), 
          DEFAULT SUBPARTITION other_regions  WITH (tablename='part_table')
          )
`
			}
			Expect(result).To(Equal(expectedResult))

		})
		It("returns a value for a subpartition template in a specific schema", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.part_table (trans_id int, date date, amount decimal(9,2), region text)
  DISTRIBUTED BY (trans_id)
  PARTITION BY RANGE (date)
  SUBPARTITION BY LIST (region)
  SUBPARTITION TEMPLATE
    ( SUBPARTITION usa VALUES ('usa'),
      SUBPARTITION asia VALUES ('asia'),
      SUBPARTITION europe VALUES ('europe'),
      DEFAULT SUBPARTITION other_regions )
  ( START (date '2014-01-01') INCLUSIVE
    END (date '2014-04-01') EXCLUSIVE
    EVERY (INTERVAL '1 month') ) `)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.part_table")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema CASCADE")
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE testschema.part_table (trans_id int, date date, amount decimal(9,2), region text)
  DISTRIBUTED BY (trans_id)
  PARTITION BY RANGE (date)
  SUBPARTITION BY LIST (region)
  SUBPARTITION TEMPLATE
    ( SUBPARTITION usa VALUES ('usa'),
      SUBPARTITION asia VALUES ('asia'),
      SUBPARTITION europe VALUES ('europe'),
      DEFAULT SUBPARTITION other_regions )
  ( START (date '2014-01-01') INCLUSIVE
    END (date '2014-04-01') EXCLUSIVE
    EVERY (INTERVAL '1 month') ) `)
			oid := testutils.OidFromObjectName(connectionPool, "testschema", "part_table", backup.TYPE_RELATION)

			backupCmdFlags.Set(utils.INCLUDE_SCHEMA, "testschema")

			results := backup.GetPartitionTemplates(connectionPool)
			Expect(results).To(HaveLen(1))
			result := results[oid]

			/*
			 * The spacing is very specific here and is output from the postgres function
			 * The only difference between the below statements is spacing
			 */
			expectedResult := ""
			if connectionPool.Version.Before("6") {
				expectedResult = `ALTER TABLE testschema.part_table 
SET SUBPARTITION TEMPLATE  
          (
          SUBPARTITION usa VALUES('usa') WITH (tablename='part_table'), 
          SUBPARTITION asia VALUES('asia') WITH (tablename='part_table'), 
          SUBPARTITION europe VALUES('europe') WITH (tablename='part_table'), 
          DEFAULT SUBPARTITION other_regions  WITH (tablename='part_table')
          )
`
			} else {
				expectedResult = `ALTER TABLE testschema.part_table 
SET SUBPARTITION TEMPLATE 
          (
          SUBPARTITION usa VALUES('usa') WITH (tablename='part_table'), 
          SUBPARTITION asia VALUES('asia') WITH (tablename='part_table'), 
          SUBPARTITION europe VALUES('europe') WITH (tablename='part_table'), 
          DEFAULT SUBPARTITION other_regions  WITH (tablename='part_table')
          )
`
			}

			Expect(result).To(Equal(expectedResult))
		})
	})
	Describe("GetTableType", func() {
		It("Returns a map when a table OF type exists", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE TYPE public.some_type AS (a text, b numeric)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TYPE public.some_type")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.some_table OF public.some_type (PRIMARY KEY (a), b WITH OPTIONS DEFAULT 1000)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.some_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "some_table", backup.TYPE_RELATION)

			result := backup.GetTableType(connectionPool)
			Expect(result).To(HaveLen(1))

			Expect(result[oid]).To(Equal("public.some_type"))
		})
		It("Returns empty map when no tables OF type exist", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.some_table (i int, j int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.some_table")

			result := backup.GetTableType(connectionPool)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("GetUnloggedTables", func() {
		It("Returns a map when an UNLOGGED table exists", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE UNLOGGED TABLE public.some_table(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.some_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "some_table", backup.TYPE_RELATION)

			result := backup.GetUnloggedTables(connectionPool)
			Expect(result).To(HaveLen(1))

			Expect(result[oid]).To(BeTrue())
		})
		It("Returns empty map when no UNLOGGED tables exist", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.some_table (i int, j int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.some_table")

			result := backup.GetUnloggedTables(connectionPool)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("GetForeignTableDefinitions", func() {
		It("Returns a map when a FOREIGN table exists", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE FOREIGN DATA WRAPPER dummy;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP FOREIGN DATA WRAPPER dummy")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SERVER sc FOREIGN DATA WRAPPER dummy;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SERVER sc")
			testhelper.AssertQueryRuns(connectionPool, `CREATE FOREIGN TABLE public.ft1 (
	c1 integer OPTIONS (param1 'val1') NOT NULL,
	c3 date
) SERVER sc OPTIONS (delimiter ',', quote '"');`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP FOREIGN TABLE public.ft1")
			oid := testutils.OidFromObjectName(connectionPool, "public", "ft1", backup.TYPE_RELATION)
			result := backup.GetForeignTableDefinitions(connectionPool)
			expectedResult := backup.ForeignTableDefinition{Oid: oid, Options: "delimiter ',',    quote '\"'", Server: "sc"}
			Expect(result).To(HaveLen(1))
			Expect(result[oid]).To(Equal(expectedResult))
		})
		It("Returns an empty map when no FOREIGN table exists", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.some_table (i int, j int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.some_table")

			result := backup.GetForeignTableDefinitions(connectionPool)
			Expect(result).To(BeEmpty())
		})
	})
	Describe("GetForeignTableRelations", func() {
		It("Returns a list with FOREIGN table", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE FOREIGN DATA WRAPPER dummy;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP FOREIGN DATA WRAPPER dummy")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SERVER sc FOREIGN DATA WRAPPER dummy;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SERVER sc")
			testhelper.AssertQueryRuns(connectionPool, `CREATE FOREIGN TABLE public.ft1 (
	c1 integer OPTIONS (param1 'val1') NOT NULL,
	c3 date
) SERVER sc OPTIONS (delimiter ',', quote '"');`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP FOREIGN TABLE public.ft1")
			result := backup.GetForeignTableRelations(connectionPool)
			Expect(result).To(HaveLen(1))
			Expect(result[0].Schema).To(Equal("public"))
			Expect(result[0].Name).To(Equal("ft1"))
		})
		It("Returns an empty list when no FOREIGN table exits", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.some_table (i int, j int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.some_table")
			result := backup.GetForeignTableRelations(connectionPool)
			Expect(result).To(HaveLen(0))
		})
	})
	Describe("GetTableStorageOptions", func() {
		It("returns an empty string when no table storage options exist ", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.simple_table(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.simple_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "simple_table", backup.TYPE_RELATION)

			result := backup.GetTableStorageOptions(connectionPool)[oid]

			Expect(result).To(Equal(""))
		})
		It("returns a value for storage options of a table ", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.ao_table(i int) with (appendonly=true)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.ao_table")
			oid := testutils.OidFromObjectName(connectionPool, "public", "ao_table", backup.TYPE_RELATION)

			result := backup.GetTableStorageOptions(connectionPool)[oid]

			Expect(result).To(Equal("appendonly=true"))
		})
	})
	Describe("GetAllSequenceRelations", func() {
		It("returns a slice of all sequences", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence START 10")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")
			testhelper.AssertQueryRuns(connectionPool, "COMMENT ON SEQUENCE public.my_sequence IS 'this is a sequence comment'")

			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema CASCADE")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE testschema.my_sequence2")

			sequences := backup.GetAllSequenceRelations(connectionPool)

			mySequence := backup.Relation{Schema: "public", Name: "my_sequence"}
			mySequence2 := backup.Relation{Schema: "testschema", Name: "my_sequence2"}

			Expect(sequences).To(HaveLen(2))
			structmatcher.ExpectStructsToMatchExcluding(&mySequence, &sequences[0], "SchemaOid", "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&mySequence2, &sequences[1], "SchemaOid", "Oid")
		})
		It("returns a slice of all sequences in a specific schema", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence START 10")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")

			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema CASCADE")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE testschema.my_sequence")
			mySequence := backup.Relation{Schema: "testschema", Name: "my_sequence"}

			backupCmdFlags.Set(utils.INCLUDE_SCHEMA, "testschema")
			sequences := backup.GetAllSequenceRelations(connectionPool)

			Expect(sequences).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&mySequence, &sequences[0], "SchemaOid", "Oid")
		})
		It("does not return sequences owned by included tables", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence START 10")

			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.seq_table(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.seq_table")
			testhelper.AssertQueryRuns(connectionPool, "ALTER SEQUENCE public.my_sequence OWNED BY public.seq_table.i")
			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.seq_table")

			sequences := backup.GetAllSequenceRelations(connectionPool)

			Expect(sequences).To(BeEmpty())
		})
		It("returns sequences owned by excluded tables if the sequence is not excluded", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence START 10")

			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.seq_table(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.seq_table")
			testhelper.AssertQueryRuns(connectionPool, "ALTER SEQUENCE public.my_sequence OWNED BY public.seq_table.i")
			mySequence := backup.Relation{Schema: "public", Name: "my_sequence"}

			backupCmdFlags.Set(utils.EXCLUDE_RELATION, "public.seq_table")
			sequences := backup.GetAllSequenceRelations(connectionPool)

			Expect(sequences).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&mySequence, &sequences[0], "SchemaOid", "Oid")
		})
		It("does not return an excluded sequence", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.sequence1 START 10")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.sequence1")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.sequence2 START 10")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.sequence2")

			sequence2 := backup.Relation{Schema: "public", Name: "sequence2"}

			backupCmdFlags.Set(utils.EXCLUDE_RELATION, "public.sequence1")
			sequences := backup.GetAllSequenceRelations(connectionPool)

			Expect(sequences).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&sequence2, &sequences[0], "SchemaOid", "Oid")
		})
		It("returns only the included sequence", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.sequence1 START 10")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.sequence1")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.sequence2 START 10")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.sequence2")

			sequence1 := backup.Relation{Schema: "public", Name: "sequence1"}
			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.sequence1")

			sequences := backup.GetAllSequenceRelations(connectionPool)

			Expect(sequences).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&sequence1, &sequences[0], "SchemaOid", "Oid")
		})
	})
	Describe("GetSequenceDefinition", func() {
		It("returns sequence information for sequence with default values", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")

			resultSequenceDef := backup.GetSequenceDefinition(connectionPool, "public.my_sequence")

			expectedSequence := backup.SequenceDefinition{Name: "my_sequence", LastVal: 1, Increment: 1, MaxVal: math.MaxInt64, MinVal: 1, CacheVal: 1}
			if connectionPool.Version.Before("5") {
				expectedSequence.LogCnt = 1 // In GPDB 4.3, sequence log count is one-indexed
			}
			if connectionPool.Version.AtLeast("6") {
				expectedSequence.StartVal = 1
			}

			structmatcher.ExpectStructsToMatch(&expectedSequence, &resultSequenceDef)
		})
		It("returns sequence information for a complex sequence", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.with_sequence(a int, b char(20))")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.with_sequence")
			testhelper.AssertQueryRuns(connectionPool,
				"CREATE SEQUENCE public.my_sequence INCREMENT BY 5 MINVALUE 20 MAXVALUE 1000 START 100 OWNED BY public.with_sequence.a")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")
			testhelper.AssertQueryRuns(connectionPool, "INSERT INTO public.with_sequence VALUES (nextval('public.my_sequence'), 'acme')")
			testhelper.AssertQueryRuns(connectionPool, "INSERT INTO public.with_sequence VALUES (nextval('public.my_sequence'), 'beta')")

			resultSequenceDef := backup.GetSequenceDefinition(connectionPool, "public.my_sequence")

			expectedSequence := backup.SequenceDefinition{Name: "my_sequence", LastVal: 105, Increment: 5, MaxVal: 1000, MinVal: 20, CacheVal: 1, IsCycled: false, IsCalled: true}
			if connectionPool.Version.Before("5") {
				expectedSequence.LogCnt = 32 // In GPDB 4.3, sequence log count is one-indexed
			} else {
				expectedSequence.LogCnt = 31 // In GPDB 5, sequence log count is zero-indexed
			}
			if connectionPool.Version.AtLeast("6") {
				expectedSequence.StartVal = 100
			}

			structmatcher.ExpectStructsToMatch(&expectedSequence, &resultSequenceDef)
		})
	})
	Describe("GetSequenceOwnerMap", func() {
		It("returns sequence information for sequences owned by columns", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.without_sequence(a int, b char(20));")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.without_sequence")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.with_sequence(a int, b char(20));")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.with_sequence")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence OWNED BY public.with_sequence.a;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")

			sequenceOwnerTables, sequenceOwnerColumns := backup.GetSequenceColumnOwnerMap(connectionPool)

			Expect(sequenceOwnerTables).To(HaveLen(1))
			Expect(sequenceOwnerColumns).To(HaveLen(1))
			Expect(sequenceOwnerTables["public.my_sequence"]).To(Equal("public.with_sequence"))
			Expect(sequenceOwnerColumns["public.my_sequence"]).To(Equal("public.with_sequence.a"))
		})
		It("does not return sequence owner columns if the owning table is not backed up", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.my_table(a int, b char(20));")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.my_table")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence OWNED BY public.my_table.a;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")

			backupCmdFlags.Set(utils.EXCLUDE_RELATION, "public.my_table")
			sequenceOwnerTables, sequenceOwnerColumns := backup.GetSequenceColumnOwnerMap(connectionPool)

			Expect(sequenceOwnerTables).To(BeEmpty())
			Expect(sequenceOwnerColumns).To(BeEmpty())

		})
		It("returns sequence owner if both table and sequence are backed up", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.my_table(a int, b char(20));")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.my_table")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence OWNED BY public.my_table.a;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.my_sequence,public.my_table")
			sequenceOwnerTables, sequenceOwnerColumns := backup.GetSequenceColumnOwnerMap(connectionPool)
			Expect(sequenceOwnerTables).To(HaveLen(1))
			Expect(sequenceOwnerColumns).To(HaveLen(1))
		})
		It("returns sequence owner if only the table is backed up", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.my_table(a int, b char(20));")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.my_table")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.my_sequence OWNED BY public.my_table.a;")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.my_sequence")

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.my_table")
			sequenceOwnerTables, sequenceOwnerColumns := backup.GetSequenceColumnOwnerMap(connectionPool)
			Expect(sequenceOwnerTables).To(HaveLen(1))
			Expect(sequenceOwnerColumns).To(HaveLen(1))
		})
	})
	Describe("GetAllSequences", func() {
		It("returns a slice of definitions for all sequences", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.seq_one START 3")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.seq_one")
			testhelper.AssertQueryRuns(connectionPool, "COMMENT ON SEQUENCE public.seq_one IS 'this is a sequence comment'")
			startValOne := int64(0)
			startValTwo := int64(0)
			if connectionPool.Version.AtLeast("6") {
				startValOne = 3
				startValTwo = 7
			}

			testhelper.AssertQueryRuns(connectionPool, "CREATE SEQUENCE public.seq_two START 7")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SEQUENCE public.seq_two")

			seqOneRelation := backup.Relation{Schema: "public", Name: "seq_one"}

			seqOneDef := backup.SequenceDefinition{Name: "seq_one", LastVal: 3, Increment: 1, MaxVal: math.MaxInt64, MinVal: 1, CacheVal: 1, StartVal: startValOne}
			seqTwoRelation := backup.Relation{Schema: "public", Name: "seq_two"}
			seqTwoDef := backup.SequenceDefinition{Name: "seq_two", LastVal: 7, Increment: 1, MaxVal: math.MaxInt64, MinVal: 1, CacheVal: 1, StartVal: startValTwo}
			if connectionPool.Version.Before("5") {
				seqOneDef.LogCnt = 1 // In GPDB 4.3, sequence log count is one-indexed
				seqTwoDef.LogCnt = 1
			}

			results := backup.GetAllSequences(connectionPool, map[string]string{})

			structmatcher.ExpectStructsToMatchExcluding(&seqOneRelation, &results[0].Relation, "SchemaOid", "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&seqOneDef, &results[0].SequenceDefinition)
			structmatcher.ExpectStructsToMatchExcluding(&seqTwoRelation, &results[1].Relation, "SchemaOid", "Oid")
			structmatcher.ExpectStructsToMatchExcluding(&seqTwoDef, &results[1].SequenceDefinition)
		})
	})
	Describe("GetViews", func() {
		var viewDef string
		BeforeEach(func() {
			if connectionPool.Version.Before("6") {
				viewDef = "SELECT 1;"
			} else {
				viewDef = " SELECT 1;"
			}
		})
		It("returns a slice for a basic view", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE VIEW public.simpleview AS SELECT 1")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP VIEW public.simpleview")

			results := backup.GetViews(connectionPool)

			view := backup.View{Oid: 1, Schema: "public", Name: "simpleview", Definition: viewDef}

			Expect(results).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&view, &results[0], "Oid")
		})
		It("returns a slice for view in a specific schema", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE VIEW public.simpleview AS SELECT 1")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP VIEW public.simpleview")
			testhelper.AssertQueryRuns(connectionPool, "CREATE SCHEMA testschema")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP SCHEMA testschema")
			testhelper.AssertQueryRuns(connectionPool, "CREATE VIEW testschema.simpleview AS SELECT 1")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP VIEW testschema.simpleview")
			backupCmdFlags.Set(utils.INCLUDE_SCHEMA, "testschema")

			results := backup.GetViews(connectionPool)

			view := backup.View{Oid: 1, Schema: "testschema", Name: "simpleview", Definition: viewDef}

			Expect(results).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&view, &results[0], "Oid")
		})
		It("returns a slice for a view with options", func() {
			testutils.SkipIfBefore6(connectionPool)
			testhelper.AssertQueryRuns(connectionPool, "CREATE VIEW public.simpleview WITH (security_barrier=true) AS SELECT 1")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP VIEW public.simpleview")

			results := backup.GetViews(connectionPool)

			view := backup.View{Oid: 1, Schema: "public", Name: "simpleview", Definition: viewDef, Options: " WITH (security_barrier=true)"}

			Expect(results).To(HaveLen(1))
			structmatcher.ExpectStructsToMatchExcluding(&view, &results[0], "Oid")
		})
	})
	Describe("GetTableInheritance", func() {
		child := backup.Relation{Schema: "public", Name: "child"}
		childOne := backup.Relation{Schema: "public", Name: "child_one"}
		childTwo := backup.Relation{Schema: "public", Name: "child_two"}
		parent := backup.Relation{Schema: "public", Name: "parent"}
		parentOne := backup.Relation{Schema: "public", Name: "parent_one"}
		parentTwo := backup.Relation{Schema: "public", Name: "parent_two"}

		It("constructs dependencies correctly if there is one table dependent on one table", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.child() INHERITS (public.parent)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.child")

			child.Oid = testutils.OidFromObjectName(connectionPool, "public", "child", backup.TYPE_RELATION)
			tables := []backup.Relation{child, parent}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(HaveLen(1))
			Expect(inheritanceMap[child.Oid]).To(ConsistOf("public.parent"))
		})
		It("constructs dependencies correctly if there are two tables dependent on one table", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.child_one() INHERITS (public.parent)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.child_one")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.child_two() INHERITS (public.parent)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.child_two")

			childOne.Oid = testutils.OidFromObjectName(connectionPool, "public", "child_one", backup.TYPE_RELATION)
			childTwo.Oid = testutils.OidFromObjectName(connectionPool, "public", "child_two", backup.TYPE_RELATION)
			tables := []backup.Relation{parent, childOne, childTwo}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(HaveLen(2))
			Expect(inheritanceMap[childOne.Oid]).To(ConsistOf("public.parent"))
			Expect(inheritanceMap[childTwo.Oid]).To(ConsistOf("public.parent"))
		})
		It("constructs dependencies correctly if there is one table dependent on two tables", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent_one(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent_one")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent_two(j int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent_two")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.child() INHERITS (public.parent_one, public.parent_two)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.child")

			child.Oid = testutils.OidFromObjectName(connectionPool, "public", "child", backup.TYPE_RELATION)
			tables := []backup.Relation{parentOne, parentTwo, child}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(HaveLen(1))
			Expect(inheritanceMap[child.Oid]).To(Equal([]string{"public.parent_one", "public.parent_two"}))
		})
		It("constructs dependencies correctly if there are no table dependencies", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent")
			tables := []backup.Relation{}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(BeEmpty())
		})
		It("constructs dependencies correctly if there are no table dependencies while filtering", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent")

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.parent")
			tables := []backup.Relation{}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(BeEmpty())
		})
		It("constructs dependencies correctly if there are two dependent tables but one is not in the backup set", func() {
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.parent(i int)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.parent")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.child_one() INHERITS (public.parent)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.child_one")
			testhelper.AssertQueryRuns(connectionPool, "CREATE TABLE public.child_two() INHERITS (public.parent)")
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.child_two")

			backupCmdFlags.Set(utils.INCLUDE_RELATION, "public.child_one")
			childOne.Oid = testutils.OidFromObjectName(connectionPool, "public", "child_one", backup.TYPE_RELATION)
			tables := []backup.Relation{childOne}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(HaveLen(1))
			Expect(inheritanceMap[childOne.Oid]).To(ConsistOf("public.parent"))
		})
		It("does not record a dependency of an external leaf partition on a parent table", func() {
			testhelper.AssertQueryRuns(connectionPool, `CREATE TABLE public.partition_table (id int, gender char(1))
		DISTRIBUTED BY (id)
		PARTITION BY LIST (gender)
		( PARTITION girls VALUES ('F'),
		  PARTITION boys VALUES ('M'),
		  DEFAULT PARTITION other );`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table")
			testhelper.AssertQueryRuns(connectionPool, `CREATE EXTERNAL WEB TABLE public.partition_table_ext_part_ (like public.partition_table_1_prt_girls)
		EXECUTE 'echo -e "2\n1"' on host
		FORMAT 'csv';`)
			defer testhelper.AssertQueryRuns(connectionPool, "DROP TABLE public.partition_table_ext_part_")
			testhelper.AssertQueryRuns(connectionPool, `ALTER TABLE public.partition_table EXCHANGE PARTITION girls WITH TABLE public.partition_table_ext_part_ WITHOUT VALIDATION;`)

			partition := backup.Relation{Schema: "public", Name: "partition_table_ext_part_"}

			partition.Oid = testutils.OidFromObjectName(connectionPool, "public", "partition_table_ext_part_", backup.TYPE_RELATION)
			tables := []backup.Relation{partition}

			inheritanceMap := backup.GetTableInheritance(connectionPool, tables)

			Expect(inheritanceMap).To(Not(HaveKey(partition.Oid)))
		})
	})
})
