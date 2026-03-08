package svelte

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semsource/source/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_ParseFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create a test Svelte file
	svelteContent := `<script lang="ts">
	import Icon from '$lib/components/shared/Icon.svelte';
	import Card from './Card.svelte';

	interface Props {
		plan: PlanWithStatus;
		onUpdate?: () => void;
	}

	let { plan, onUpdate = $bindable() }: Props = $props();

	const pipeline = $derived(derivePlanPipeline(plan));
	const isDraft = $derived(!plan.approved);
	let count = $state(0);

	async function handlePromote(e: Event) {
		e.preventDefault();
		await plansStore.promote(plan.slug);
	}

	$effect(() => {
		console.log('Plan changed:', plan);
	});
</script>

<div class="plan-card">
	<Card {plan}>
		<Icon name="check" />
		<span>{count}</span>
	</Card>
</div>

<style>
	.plan-card {
		padding: 1rem;
	}
</style>
`

	testFile := filepath.Join(tmpDir, "PlanCard.svelte")
	err := os.WriteFile(testFile, []byte(svelteContent), 0o644)
	require.NoError(t, err)

	// Parse the file
	parser := NewParser("testorg", "testproject", tmpDir)
	result, err := parser.ParseFile(context.Background(), testFile)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify file entity
	assert.Equal(t, "PlanCard.svelte", filepath.Base(result.Path))
	assert.NotEmpty(t, result.Hash)

	// Verify imports were extracted
	assert.Contains(t, result.Imports, "$lib/components/shared/Icon.svelte")
	assert.Contains(t, result.Imports, "./Card.svelte")

	// Find the component entity
	var componentEntity *ast.CodeEntity
	for _, entity := range result.Entities {
		if entity.Type == ast.TypeComponent {
			componentEntity = entity
			break
		}
	}
	require.NotNil(t, componentEntity, "Component entity should be created")
	assert.Equal(t, "PlanCard", componentEntity.Name)
	assert.Equal(t, "typescript", componentEntity.Language, "Language should be the actual script language")
	assert.Equal(t, "svelte", componentEntity.Framework, "Framework should be svelte")

	// Verify rune info is in DocComment
	assert.Contains(t, componentEntity.DocComment, "Props:")
	assert.Contains(t, componentEntity.DocComment, "plan")
	assert.Contains(t, componentEntity.DocComment, "Derived:")
	assert.Contains(t, componentEntity.DocComment, "pipeline")
	assert.Contains(t, componentEntity.DocComment, "State:")
	assert.Contains(t, componentEntity.DocComment, "count")
	assert.Contains(t, componentEntity.DocComment, "Effects:")

	// Verify component references
	assert.NotEmpty(t, componentEntity.References, "Should have component references")
}

func TestParser_ParseFileSimple(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal Svelte file
	svelteContent := `<script>
	let name = 'world';
</script>

<h1>Hello {name}!</h1>
`

	testFile := filepath.Join(tmpDir, "Hello.svelte")
	err := os.WriteFile(testFile, []byte(svelteContent), 0o644)
	require.NoError(t, err)

	parser := NewParser("org", "proj", tmpDir)
	result, err := parser.ParseFile(context.Background(), testFile)
	require.NoError(t, err)

	assert.Equal(t, "Hello.svelte", filepath.Base(result.Path))

	// Find component entity
	var comp *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeComponent {
			comp = e
			break
		}
	}
	require.NotNil(t, comp)
	assert.Equal(t, "Hello", comp.Name)
}

func TestParser_ParseDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	libDir := filepath.Join(tmpDir, "lib", "components")
	err := os.MkdirAll(libDir, 0o755)
	require.NoError(t, err)

	// Create multiple Svelte files
	files := map[string]string{
		"Button.svelte": `<script>
	let { label } = $props();
</script>
<button>{label}</button>`,
		"Card.svelte": `<script>
	let { title } = $props();
</script>
<div class="card"><h2>{title}</h2><slot /></div>`,
	}

	for name, content := range files {
		path := filepath.Join(libDir, name)
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	parser := NewParser("org", "proj", tmpDir)
	results, err := parser.ParseDirectory(context.Background(), tmpDir)
	require.NoError(t, err)

	assert.Len(t, results, 2, "Should parse both Svelte files")

	// Verify both files were parsed
	parsedFiles := make(map[string]bool)
	for _, result := range results {
		parsedFiles[filepath.Base(result.Path)] = true
	}
	assert.True(t, parsedFiles["Button.svelte"])
	assert.True(t, parsedFiles["Card.svelte"])
}

func TestParser_SkipsNodeModules(t *testing.T) {
	tmpDir := t.TempDir()

	// Create node_modules directory with Svelte file
	nodeModulesDir := filepath.Join(tmpDir, "node_modules", "some-pkg")
	err := os.MkdirAll(nodeModulesDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(nodeModulesDir, "Test.svelte"), []byte("<h1>Test</h1>"), 0o644)
	require.NoError(t, err)

	// Create a regular Svelte file
	err = os.WriteFile(filepath.Join(tmpDir, "App.svelte"), []byte("<h1>App</h1>"), 0o644)
	require.NoError(t, err)

	parser := NewParser("org", "proj", tmpDir)
	results, err := parser.ParseDirectory(context.Background(), tmpDir)
	require.NoError(t, err)

	assert.Len(t, results, 1, "Should only parse App.svelte, not node_modules content")
	assert.Equal(t, "App.svelte", filepath.Base(results[0].Path))
}

func TestExtractRunes_Props(t *testing.T) {
	script := `
	let { plan, onUpdate = $bindable() }: Props = $props();
	`

	info := extractRunes([]byte(script))

	require.Len(t, info.Props, 2)
	assert.Equal(t, "plan", info.Props[0].Name)
	assert.Equal(t, "onUpdate", info.Props[1].Name)
	assert.True(t, info.Props[1].Bindable)
}

func TestExtractRunes_State(t *testing.T) {
	script := `
	let count = $state(0);
	let name = $state('world');
	let items = $state<string[]>([]);
	`

	info := extractRunes([]byte(script))

	require.Len(t, info.State, 3)
	assert.Equal(t, "count", info.State[0].Name)
	assert.Equal(t, "0", info.State[0].InitialValue)
	assert.Equal(t, "name", info.State[1].Name)
	assert.Equal(t, "'world'", info.State[1].InitialValue)
	assert.Equal(t, "items", info.State[2].Name)
}

func TestExtractRunes_Derived(t *testing.T) {
	script := `
	const pipeline = $derived(derivePlanPipeline(plan));
	const isDraft = $derived(!plan.approved);
	let count = $derived(items.length);
	`

	info := extractRunes([]byte(script))

	require.Len(t, info.Derived, 3)
	assert.Equal(t, "pipeline", info.Derived[0].Name)
	assert.Contains(t, info.Derived[0].Expression, "derivePlanPipeline")
	assert.Equal(t, "isDraft", info.Derived[1].Name)
	assert.Equal(t, "count", info.Derived[2].Name)
}

func TestExtractRunes_Effects(t *testing.T) {
	script := `
	$effect(() => {
		console.log('count changed:', count);
	});

	$effect(() => {
		document.title = name;
	});
	`

	info := extractRunes([]byte(script))

	assert.Len(t, info.Effects, 2)
}

func TestRuneInfo_ToDocComment(t *testing.T) {
	info := &RuneInfo{
		Props: []PropInfo{
			{Name: "plan", Type: "Plan"},
			{Name: "onUpdate", Bindable: true},
		},
		State: []StateInfo{
			{Name: "count"},
		},
		Derived: []DerivedInfo{
			{Name: "pipeline"},
			{Name: "isDraft"},
		},
		Effects: []EffectInfo{
			{},
			{},
		},
	}

	comment := info.ToDocComment()

	assert.Contains(t, comment, "Props: plan: Plan, onUpdate (bindable)")
	assert.Contains(t, comment, "State: count")
	assert.Contains(t, comment, "Derived: pipeline, isDraft")
	assert.Contains(t, comment, "Effects: 2")
}

func TestIsComponentName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Button", true},
		{"PlanCard", true},
		{"div", false},
		{"span", false},
		{"button", false},
		{"MyComponent", true},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isComponentName(tc.name))
		})
	}
}

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/path/to/Button.svelte", "Button"},
		{"/path/to/PlanCard.svelte", "PlanCard"},
		{"MyComponent.svelte", "MyComponent"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractComponentName(tc.path))
		})
	}
}

func TestParser_Integration_RealComponent(t *testing.T) {
	// This test uses a more realistic component structure
	tmpDir := t.TempDir()

	svelteContent := `<script lang="ts">
	import Icon from '$lib/components/shared/Icon.svelte';
	import PipelineIndicator from './PipelineIndicator.svelte';
	import ModeIndicator from './ModeIndicator.svelte';
	import AgentBadge from './AgentBadge.svelte';
	import { derivePlanPipeline, type PlanWithStatus } from '$lib/types/plan';
	import { plansStore } from '$lib/stores/plans.svelte';
	import { questionsStore } from '$lib/stores/questions.svelte';

	interface Props {
		plan: PlanWithStatus;
	}

	let { plan }: Props = $props();

	const pipeline = $derived(derivePlanPipeline(plan));
	const isDraft = $derived(!plan.approved);
	const hasRejection = $derived(
		(plan.active_loops ?? []).some((l) => l.current_task_id) &&
			plansStore.getTasks(plan.slug).some((t) => t.rejection)
	);

	const planLoopIds = $derived((plan.active_loops ?? []).map((l) => l.loop_id));
	const questionCount = $derived(
		questionsStore.pending.filter(
			(q) => q.blocked_loop_id && planLoopIds.includes(q.blocked_loop_id)
		).length
	);

	async function handlePromote(e: Event) {
		e.preventDefault();
		e.stopPropagation();
		await plansStore.promote(plan.slug);
	}

	async function handleExecute(e: Event) {
		e.preventDefault();
		e.stopPropagation();
		await plansStore.execute(plan.slug);
	}
</script>

<a href="/plans/{plan.slug}" class="plan-card">
	<div class="card-header">
		<h3>{plan.slug}</h3>
		<ModeIndicator approved={plan.approved} compact />
	</div>

	{#if plan.approved}
		<PipelineIndicator plan={pipeline.plan} tasks={pipeline.tasks} execute={pipeline.execute} />
	{/if}

	{#if (plan.active_loops ?? []).length > 0}
		<div class="agents-row">
			{#each plan.active_loops ?? [] as loop}
				<AgentBadge role={loop.role} model={loop.model} state={loop.state} />
			{/each}
		</div>
	{/if}

	{#if isDraft && plan.goal}
		<button class="promote-btn" onclick={handlePromote}>
			<Icon name="check" size={14} />
			Approve Plan
		</button>
	{/if}
</a>

<style>
	.plan-card {
		padding: var(--space-4);
	}
</style>
`

	testFile := filepath.Join(tmpDir, "PlanCard.svelte")
	err := os.WriteFile(testFile, []byte(svelteContent), 0o644)
	require.NoError(t, err)

	parser := NewParser("testorg", "ui", tmpDir)
	result, err := parser.ParseFile(context.Background(), testFile)
	require.NoError(t, err)

	// Verify imports
	expectedImports := []string{
		"$lib/components/shared/Icon.svelte",
		"./PipelineIndicator.svelte",
		"./ModeIndicator.svelte",
		"./AgentBadge.svelte",
		"$lib/types/plan",
		"$lib/stores/plans.svelte",
		"$lib/stores/questions.svelte",
	}
	for _, imp := range expectedImports {
		assert.Contains(t, result.Imports, imp, "Should contain import: %s", imp)
	}

	// Find component entity
	var comp *ast.CodeEntity
	for _, e := range result.Entities {
		if e.Type == ast.TypeComponent {
			comp = e
			break
		}
	}
	require.NotNil(t, comp)

	// Verify runes in doc comment
	assert.Contains(t, comp.DocComment, "Props:")
	assert.Contains(t, comp.DocComment, "plan")
	assert.Contains(t, comp.DocComment, "Derived:")
	assert.Contains(t, comp.DocComment, "pipeline")
	assert.Contains(t, comp.DocComment, "isDraft")
	assert.Contains(t, comp.DocComment, "hasRejection")
	assert.Contains(t, comp.DocComment, "questionCount")

	// Verify functions were extracted
	var funcs []string
	for _, e := range result.Entities {
		if e.Type == ast.TypeFunction {
			funcs = append(funcs, e.Name)
		}
	}
	assert.Contains(t, funcs, "handlePromote")
	assert.Contains(t, funcs, "handleExecute")
}
