export namespace cli {
	
	export class ReviewResult {
	    facts: facts.Facts;
	    policies: policy.Result[];
	    passed: boolean;
	    fail: number;
	    warn: number;
	    pass: number;
	    duration_ms: number;
	    started_at: string;
	    project_root: string;
	    baseline_used?: string[];
	    trend?: report.Trend;
	    artifact_path?: string;
	    report_path?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.facts = this.convertValues(source["facts"], facts.Facts);
	        this.policies = this.convertValues(source["policies"], policy.Result);
	        this.passed = source["passed"];
	        this.fail = source["fail"];
	        this.warn = source["warn"];
	        this.pass = source["pass"];
	        this.duration_ms = source["duration_ms"];
	        this.started_at = source["started_at"];
	        this.project_root = source["project_root"];
	        this.baseline_used = source["baseline_used"];
	        this.trend = this.convertValues(source["trend"], report.Trend);
	        this.artifact_path = source["artifact_path"];
	        this.report_path = source["report_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace facts {
	
	export class LatestRun {
	    conclusion: string;
	    head_sha: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new LatestRun(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conclusion = source["conclusion"];
	        this.head_sha = source["head_sha"];
	        this.name = source["name"];
	    }
	}
	export class ActionsFacts {
	    latest: LatestRun;
	    recent_total: number;
	    recent_success_count: number;
	    recent_success_ratio: number;
	
	    static createFrom(source: any = {}) {
	        return new ActionsFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latest = this.convertValues(source["latest"], LatestRun);
	        this.recent_total = source["recent_total"];
	        this.recent_success_count = source["recent_success_count"];
	        this.recent_success_ratio = source["recent_success_ratio"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CheckResult {
	    ok: boolean;
	    skipped: boolean;
	    skip_reason: string;
	    output?: string;
	    duration_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.skip_reason = source["skip_reason"];
	        this.output = source["output"];
	        this.duration_ms = source["duration_ms"];
	    }
	}
	export class CountFacts {
	    open_critical: number;
	    open_high: number;
	    open_moderate: number;
	    open_total: number;
	
	    static createFrom(source: any = {}) {
	        return new CountFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.open_critical = source["open_critical"];
	        this.open_high = source["open_high"];
	        this.open_moderate = source["open_moderate"];
	        this.open_total = source["open_total"];
	    }
	}
	export class DocsFacts {
	    readme: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DocsFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.readme = source["readme"];
	    }
	}
	export class GitHubFacts {
	    found: boolean;
	    skip_reason?: string;
	    repo?: string;
	    actions: ActionsFacts;
	    code_scanning: CountFacts;
	    dependabot: CountFacts;
	    secret_scanning: CountFacts;
	
	    static createFrom(source: any = {}) {
	        return new GitHubFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.skip_reason = source["skip_reason"];
	        this.repo = source["repo"];
	        this.actions = this.convertValues(source["actions"], ActionsFacts);
	        this.code_scanning = this.convertValues(source["code_scanning"], CountFacts);
	        this.dependabot = this.convertValues(source["dependabot"], CountFacts);
	        this.secret_scanning = this.convertValues(source["secret_scanning"], CountFacts);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FlutterFacts {
	    found: boolean;
	    analyze: CheckResult;
	    test: CheckResult;
	
	    static createFrom(source: any = {}) {
	        return new FlutterFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.analyze = this.convertValues(source["analyze"], CheckResult);
	        this.test = this.convertValues(source["test"], CheckResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WebTSFacts {
	    found: boolean;
	    dir?: string;
	    typecheck: CheckResult;
	    lint: CheckResult;
	    test: CheckResult;
	    api_calls: string[];
	
	    static createFrom(source: any = {}) {
	        return new WebTSFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.dir = source["dir"];
	        this.typecheck = this.convertValues(source["typecheck"], CheckResult);
	        this.lint = this.convertValues(source["lint"], CheckResult);
	        this.test = this.convertValues(source["test"], CheckResult);
	        this.api_calls = source["api_calls"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RouteFacts {
	    domain: string;
	    method: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new RouteFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.domain = source["domain"];
	        this.method = source["method"];
	        this.path = source["path"];
	    }
	}
	export class ImportEdge {
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportEdge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class ImportGraph {
	    edges: ImportEdge[];
	
	    static createFrom(source: any = {}) {
	        return new ImportGraph(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.edges = this.convertValues(source["edges"], ImportEdge);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FmtFacts {
	    ok: boolean;
	    drift: string[];
	
	    static createFrom(source: any = {}) {
	        return new FmtFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.drift = source["drift"];
	    }
	}
	export class GoFacts {
	    found: boolean;
	    go_version?: string;
	    build: CheckResult;
	    vet: CheckResult;
	    test: CheckResult;
	    fmt: FmtFacts;
	    lint: CheckResult;
	    import_graph: ImportGraph;
	    routes: RouteFacts[];
	
	    static createFrom(source: any = {}) {
	        return new GoFacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.go_version = source["go_version"];
	        this.build = this.convertValues(source["build"], CheckResult);
	        this.vet = this.convertValues(source["vet"], CheckResult);
	        this.test = this.convertValues(source["test"], CheckResult);
	        this.fmt = this.convertValues(source["fmt"], FmtFacts);
	        this.lint = this.convertValues(source["lint"], CheckResult);
	        this.import_graph = this.convertValues(source["import_graph"], ImportGraph);
	        this.routes = this.convertValues(source["routes"], RouteFacts);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Project {
	    root: string;
	    types: string[];
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.types = source["types"];
	    }
	}
	export class Facts {
	    schema_version: number;
	    generated_at: string;
	    project: Project;
	    go: GoFacts;
	    web_ts: WebTSFacts;
	    flutter: FlutterFacts;
	    github: GitHubFacts;
	    docs: DocsFacts;
	
	    static createFrom(source: any = {}) {
	        return new Facts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema_version = source["schema_version"];
	        this.generated_at = source["generated_at"];
	        this.project = this.convertValues(source["project"], Project);
	        this.go = this.convertValues(source["go"], GoFacts);
	        this.web_ts = this.convertValues(source["web_ts"], WebTSFacts);
	        this.flutter = this.convertValues(source["flutter"], FlutterFacts);
	        this.github = this.convertValues(source["github"], GitHubFacts);
	        this.docs = this.convertValues(source["docs"], DocsFacts);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	

}

export namespace policy {
	
	export class Policy {
	    id: string;
	    dimension: string;
	    severity: string;
	    expr: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Policy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.dimension = source["dimension"];
	        this.severity = source["severity"];
	        this.expr = source["expr"];
	        this.message = source["message"];
	    }
	}
	export class Result {
	    policy: Policy;
	    passed: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.policy = this.convertValues(source["policy"], Policy);
	        this.passed = source["passed"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace report {
	
	export class Trend {
	    New: string[];
	    Resolved: string[];
	
	    static createFrom(source: any = {}) {
	        return new Trend(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.New = source["New"];
	        this.Resolved = source["Resolved"];
	    }
	}

}

