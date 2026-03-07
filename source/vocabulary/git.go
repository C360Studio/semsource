// Package source provides vocabulary predicates for source entities.
// This file defines git decision predicates for tracking agent decisions
// through git commit history - the "git-as-memory" pattern.
package source

import "github.com/c360studio/semstreams/vocabulary"

// Git decision predicates track agent decisions at the file level.
// Each file changed in a commit creates a decision entity that records
// the what, why, and context of the change.
const (
	// DecisionType is the decision category from conventional commit prefix.
	// Values: feat, fix, refactor, docs, test, chore, perf, ci, build, revert
	DecisionType = "source.git.decision.type"

	// DecisionFile is the path of the file that was changed.
	DecisionFile = "source.git.decision.file"

	// DecisionCommit is the git commit hash.
	DecisionCommit = "source.git.decision.commit"

	// DecisionMessage is the commit message.
	DecisionMessage = "source.git.decision.message"

	// DecisionBranch is the branch where the commit was made.
	DecisionBranch = "source.git.decision.branch"

	// DecisionAgent is the agent ID that made the commit (if semspec-driven).
	DecisionAgent = "source.git.decision.agent"

	// DecisionLoop is the agent loop ID that made the commit (if semspec-driven).
	DecisionLoop = "source.git.decision.loop"

	// DecisionProject is the project entity ID.
	DecisionProject = "source.git.decision.project"

	// DecisionTimestamp is when the commit was made (RFC3339).
	DecisionTimestamp = "source.git.decision.timestamp"

	// DecisionRepository is the repository URL or path.
	DecisionRepository = "source.git.decision.repository"

	// DecisionOperation is the type of file operation.
	// Values: add, modify, delete, rename
	DecisionOperation = "source.git.decision.operation"
)

// DecisionTypeValue represents the decision category values.
type DecisionTypeValue string

const (
	// DecisionTypeFeat is a new feature.
	DecisionTypeFeat DecisionTypeValue = "feat"

	// DecisionTypeFix is a bug fix.
	DecisionTypeFix DecisionTypeValue = "fix"

	// DecisionTypeRefactor is a code refactoring.
	DecisionTypeRefactor DecisionTypeValue = "refactor"

	// DecisionTypeDocs is documentation only changes.
	DecisionTypeDocs DecisionTypeValue = "docs"

	// DecisionTypeTest is adding or correcting tests.
	DecisionTypeTest DecisionTypeValue = "test"

	// DecisionTypeChore is maintenance tasks.
	DecisionTypeChore DecisionTypeValue = "chore"

	// DecisionTypePerf is performance improvements.
	DecisionTypePerf DecisionTypeValue = "perf"

	// DecisionTypeCI is CI/CD configuration changes.
	DecisionTypeCI DecisionTypeValue = "ci"

	// DecisionTypeBuild is build system or external dependencies.
	DecisionTypeBuild DecisionTypeValue = "build"

	// DecisionTypeRevert is reverting a previous commit.
	DecisionTypeRevert DecisionTypeValue = "revert"

	// DecisionTypeStyle is code style changes (formatting, etc).
	DecisionTypeStyle DecisionTypeValue = "style"
)

// FileOperationType represents the type of file operation.
type FileOperationType string

const (
	// FileOperationAdd is a new file.
	FileOperationAdd FileOperationType = "add"

	// FileOperationModify is a modified file.
	FileOperationModify FileOperationType = "modify"

	// FileOperationDelete is a deleted file.
	FileOperationDelete FileOperationType = "delete"

	// FileOperationRename is a renamed file.
	FileOperationRename FileOperationType = "rename"
)

// Git decision class IRIs for RDF mapping.
const (
	// ClassDecision represents a git-tracked decision entity.
	ClassDecision = Namespace + "Decision"

	// ClassFileDecision represents a per-file decision.
	ClassFileDecision = Namespace + "FileDecision"
)

func init() {
	// Register decision type predicate
	vocabulary.Register(DecisionType,
		vocabulary.WithDescription("Decision category from conventional commit prefix (feat, fix, refactor, etc.)"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionType"))

	// Register decision file predicate
	vocabulary.Register(DecisionFile,
		vocabulary.WithDescription("Path of the file that was changed"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionFile"))

	// Register decision commit predicate
	vocabulary.Register(DecisionCommit,
		vocabulary.WithDescription("Git commit hash"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionCommit"))

	// Register decision message predicate
	vocabulary.Register(DecisionMessage,
		vocabulary.WithDescription("Git commit message"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionMessage"))

	// Register decision branch predicate
	vocabulary.Register(DecisionBranch,
		vocabulary.WithDescription("Git branch where the commit was made"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionBranch"))

	// Register decision agent predicate
	vocabulary.Register(DecisionAgent,
		vocabulary.WithDescription("Agent ID that made the commit (if semspec-driven)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(vocabulary.ProvWasAttributedTo))

	// Register decision loop predicate
	vocabulary.Register(DecisionLoop,
		vocabulary.WithDescription("Agent loop ID that made the commit (if semspec-driven)"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(Namespace+"decisionLoop"))

	// Register decision project predicate
	vocabulary.Register(DecisionProject,
		vocabulary.WithDescription("Project entity ID the decision belongs to"),
		vocabulary.WithDataType("entity_id"),
		vocabulary.WithIRI(Namespace+"decisionProject"))

	// Register decision timestamp predicate
	vocabulary.Register(DecisionTimestamp,
		vocabulary.WithDescription("When the commit was made (RFC3339)"),
		vocabulary.WithDataType("datetime"),
		vocabulary.WithIRI(vocabulary.ProvGeneratedAtTime))

	// Register decision repository predicate
	vocabulary.Register(DecisionRepository,
		vocabulary.WithDescription("Repository URL or path where the decision was made"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionRepository"))

	// Register decision operation predicate
	vocabulary.Register(DecisionOperation,
		vocabulary.WithDescription("Type of file operation: add, modify, delete, rename"),
		vocabulary.WithDataType("string"),
		vocabulary.WithIRI(Namespace+"decisionOperation"))
}
