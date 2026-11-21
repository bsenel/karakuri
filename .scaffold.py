def ins(path, anchor, text):
    s = open(path).read()
    assert anchor in s, path
    s = s.replace(anchor, text + anchor, 1)
    open(path, 'w').write(s)

ins('internal/platform/tools/versioncontrol/adapter.go', "	// EditPRBody replaces a pull request's description with body.", """	// ListClosedPRs returns up to limit of the repository's most recently
	// closed pull requests that were not merged, newest first. An adapter
	// whose forge cannot returns an error wrapping ErrUnsupported.
	ListClosedPRs(ctx context.Context, repo string, limit int) ([]PRSummary, error)
""")
ins('internal/platform/tools/versioncontrol/noop.go', 'func (n *NoOp) EditPRBody(', """func (n *NoOp) ListClosedPRs(_ context.Context, _ string, _ int) ([]PRSummary, error) {
	return nil, fmt.Errorf("noop: closed pull requests: %w", ErrUnsupported)
}

""")
ins('internal/platform/tools/versioncontrol/github.go', "// EditPRBody replaces a pull request's description.", """// ListClosedPRs is a stub until the production slice lands.
func (g *GitHub) ListClosedPRs(ctx context.Context, repo string, limit int) ([]PRSummary, error) {
	return nil, fmt.Errorf("github: closed pull requests: not implemented")
}

""")
stub = """func (%s) ListClosedPRs(context.Context, string, int) ([]versioncontrol.PRSummary, error) {
	return nil, versioncontrol.ErrUnsupported
}

"""
for p, r in [('internal/feature/loop/writepath_test.go', 'v *recordingVC'), ('domains/software/provenance_test.go', 's *vcStub'), ('domains/software/stream_verifiers_test.go', 'f *fakeForge'), ('domains/software/verify_env_test.go', 'w *windowVC')]:
    ins(p, 'func (%s) ListMergedPRs(' % r, stub % r)
