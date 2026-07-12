export namespace main {
	
	export class ClaudeDeepSeekRequest {
	    apiKey: string;
	    path: string;
	    mainModel: string;
	    haikuModel: string;
	    effortLevel: string;
	    setSystemEnv: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClaudeDeepSeekRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiKey = source["apiKey"];
	        this.path = source["path"];
	        this.mainModel = source["mainModel"];
	        this.haikuModel = source["haikuModel"];
	        this.effortLevel = source["effortLevel"];
	        this.setSystemEnv = source["setSystemEnv"];
	    }
	}
	export class ConnectionTestResult {
	    ok: boolean;
	    message: string;
	    endpoint: string;
	    statusCode: number;
	    latencyMs: number;
	    modelCount: number;
	    sample: string[];
	    error?: string;
	    apiFormat?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionTestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.message = source["message"];
	        this.endpoint = source["endpoint"];
	        this.statusCode = source["statusCode"];
	        this.latencyMs = source["latencyMs"];
	        this.modelCount = source["modelCount"];
	        this.sample = source["sample"];
	        this.error = source["error"];
	        this.apiFormat = source["apiFormat"];
	    }
	}
	export class FetchModelItem {
	    id: string;
	    name: string;
	    ownedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new FetchModelItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.ownedBy = source["ownedBy"];
	    }
	}
	export class ModelApplyRequest {
	    kind: string;
	    path: string;
	    model: string;
	    provider: string;
	    baseUrl: string;
	    apiKey: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelApplyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.model = source["model"];
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.name = source["name"];
	    }
	}
	export class ModelOption {
	    id: string;
	    name: string;
	    provider: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	    }
	}
	export class ProviderModel {
	    id: string;
	    name: string;
	    enabled: boolean;
	    isDefault: boolean;
	    ownedBy?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.isDefault = source["isDefault"];
	        this.ownedBy = source["ownedBy"];
	    }
	}
	export class Provider {
	    id: string;
	    name: string;
	    baseUrl: string;
	    apiKey: string;
	    color: string;
	    apiFormat: string;
	    models: ProviderModel[];
	
	    static createFrom(source: any = {}) {
	        return new Provider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.baseUrl = source["baseUrl"];
	        this.apiKey = source["apiKey"];
	        this.color = source["color"];
	        this.apiFormat = source["apiFormat"];
	        this.models = this.convertValues(source["models"], ProviderModel);
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
	
	export class ProxyConfig {
	    enabled: boolean;
	    host: string;
	    port: number;
	    autoStart: boolean;
	    listenKey: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.autoStart = source["autoStart"];
	        this.listenKey = source["listenKey"];
	    }
	}
	export class ProxyStatus {
	    running: boolean;
	    baseUrl: string;
	    host: string;
	    port: number;
	    autoStart: boolean;
	    listenKey: string;
	    lastError: string;
	    logs: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.baseUrl = source["baseUrl"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.autoStart = source["autoStart"];
	        this.listenKey = source["listenKey"];
	        this.lastError = source["lastError"];
	        this.logs = source["logs"];
	    }
	}
	export class SystemInfo {
	    os: string;
	    arch: string;
	    homeDir: string;
	    pathSep: string;
	    fileManager: string;
	    revealLabel: string;
	    platformName: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.homeDir = source["homeDir"];
	        this.pathSep = source["pathSep"];
	        this.fileManager = source["fileManager"];
	        this.revealLabel = source["revealLabel"];
	        this.platformName = source["platformName"];
	    }
	}
	export class ToolConfigStatus {
	    kind: string;
	    name: string;
	    path: string;
	    found: boolean;
	    exists: boolean;
	    model: string;
	    modelProvider: string;
	    searchPaths: string[];
	    candidates: ModelOption[];
	    source: string;
	    message: string;
	    os: string;
	    hasDefaultBackup: boolean;
	    defaultBackupPath: string;
	    defaultBackupAt: string;
	    defaultBackupOrigin: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolConfigStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.found = source["found"];
	        this.exists = source["exists"];
	        this.model = source["model"];
	        this.modelProvider = source["modelProvider"];
	        this.searchPaths = source["searchPaths"];
	        this.candidates = this.convertValues(source["candidates"], ModelOption);
	        this.source = source["source"];
	        this.message = source["message"];
	        this.os = source["os"];
	        this.hasDefaultBackup = source["hasDefaultBackup"];
	        this.defaultBackupPath = source["defaultBackupPath"];
	        this.defaultBackupAt = source["defaultBackupAt"];
	        this.defaultBackupOrigin = source["defaultBackupOrigin"];
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

