export const meta = {
  name: 'flutter-app-crew',
  description: 'Flutter app crew: engineer builds, security audits, QA gates, release manages version and withdrawal plan',
  phases: [
    { title: 'Engineer', detail: 'Flutter mobile engineer builds with SDK-aligned deps, secure storage, recycled lists' },
    { title: 'Security', detail: 'Security reviewer audits threat model, secrets, tamper resistance' },
    { title: 'QA', detail: 'QA automation lead runs end-to-end journeys, flaky test gate' },
    { title: 'Release', detail: 'Release manager derives version, writes changelog, documents withdrawal plan' },
  ],
};

const MEMBERS = [
  { key: 'engineer', agent: 'flutter-mobile-engineer', phase: 'Engineer' },
  { key: 'security', agent: 'security-reviewer', phase: 'Security' },
  { key: 'qa', agent: 'qa-automation-lead', phase: 'QA' },
  { key: 'release', agent: 'release-manager', phase: 'Release' },
];

async function runPhase(member, task, context) {
  return agent(
    `${member.phase}: ${task}`,
    {
      label: `crew:${member.key}`,
      phase: member.phase,
      schema: z.object({
        findings: z.array(z.object({
          file: z.string(),
          line: z.number().optional(),
          severity: z.enum(['critical', 'high', 'medium', 'low', 'info']),
          message: z.string(),
          fix: z.string().optional(),
        })),
        approvals: z.array(z.string()),
        blockers: z.array(z.string()),
        nextSteps: z.array(z.string()),
      }),
    }
  ).then(r => ({ member: member.key, ...r }));
}

async function engineerPhase(context) {
  const { appPath, target } = context.args;
  return runPhase(MEMBERS[0],
    `Build Flutter app at ${appPath} for ${target}.
    Verify: SDK-aligned dependencies (flutter_riverpod, go_router, sqflite, http),
    secure storage (flutter_secure_storage), recycled lists (ListView.builder),
    no secrets in bundle (--obfuscate --split-debug-info), analysis_options.yaml with very_good_analysis pinned.`,
    context
  );
}

async function securityPhase(context) {
  const { appPath } = context.args;
  const engineerResult = context.results.engineer;
  return runPhase(MEMBERS[1],
    `Audit Flutter app at ${appPath} for security issues.
    Focus: threat model (prompt injection, ingestion endpoints, CLI secrets, sync data),
    secrets never in shared_preferences/--dart-define, tamper resistance (--obfuscate),
    network security (TLS, certificate pinning), sync one-directional (local→remote only).
    Engineer's work: ${JSON.stringify(engineerResult?.approvals || [])}`,
    context
  );
}

async function qaPhase(context) {
  const { appPath } = context.args;
  const engineerResult = context.results.engineer;
  const securityResult = context.results.security;
  return runPhase(MEMBERS[2],
    `Run QA gate for Flutter app at ${appPath}.
    Execute: flutter analyze (0 errors), flutter test (all pass), flutter build web --release.
    End-to-end journeys on emulator: add note, search, delete, pin, sync, settings config.
    Flaky test = failed test. Accessibility: labeledTapTarget, androidTapTarget, iOSTapTarget, textContrast.
    Engineer approvals: ${JSON.stringify(engineerResult?.approvals || [])}
    Security findings: ${JSON.stringify(securityResult?.findings || [])}`,
    context
  );
}

async function releasePhase(context) {
  const { appPath, version } = context.args;
  const engineerResult = context.results.engineer;
  const securityResult = context.results.security;
  const qaResult = context.results.qa;
  return runPhase(MEMBERS[3],
    `Prepare release for Flutter app at ${appPath} version ${version}.
    Derive version from history (semver), write CHANGELOG.md with Conventional Commits,
    document withdrawal plan for store build and OTA update (how to rollback).
    Engineer: ${JSON.stringify(engineerResult?.approvals || [])}
    Security: ${JSON.stringify(securityResult?.approvals || [])}
    QA gate: ${JSON.stringify(qaResult?.approvals || [])}`,
    context
  );
}

const results = await pipeline(
  MEMBERS,
  m => m.phase === 'Engineer' ? engineerPhase(context) :
       m.phase === 'Security' ? securityPhase(context) :
       m.phase === 'QA' ? qaPhase(context) :
       releasePhase(context),
);

const allApproved = results.every(r => r.approvals?.length > 0 && r.blockers?.length === 0);
const allFindings = results.flatMap(r => r.findings || []);

return {
  approved: allApproved,
  results,
  findings: allFindings,
  version: context.args.version,
  withdrawalPlan: results.find(r => r.member === 'release')?.findings?.find(f => f.message.includes('withdrawal'))?.fix,
};