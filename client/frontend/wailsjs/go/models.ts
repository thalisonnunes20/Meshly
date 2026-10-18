export namespace main {
	
	export class Peer {
	    public_key: string;
	    nickname: string;
	    virtual_ip: string;
	    room_id: string;
	    is_online: boolean;
	    ping_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new Peer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.public_key = source["public_key"];
	        this.nickname = source["nickname"];
	        this.virtual_ip = source["virtual_ip"];
	        this.room_id = source["room_id"];
	        this.is_online = source["is_online"];
	        this.ping_ms = source["ping_ms"];
	    }
	}
	export class RoomState {
	    room_id: string;
	    room_name: string;
	    subnet_cidr: string;
	    virtual_ip: string;
	    virtual_cidr: string;
	    server_endpoint: string;
	    server_public_key: string;
	    peers: Peer[];
	
	    static createFrom(source: any = {}) {
	        return new RoomState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.room_id = source["room_id"];
	        this.room_name = source["room_name"];
	        this.subnet_cidr = source["subnet_cidr"];
	        this.virtual_ip = source["virtual_ip"];
	        this.virtual_cidr = source["virtual_cidr"];
	        this.server_endpoint = source["server_endpoint"];
	        this.server_public_key = source["server_public_key"];
	        this.peers = this.convertValues(source["peers"], Peer);
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
	export class ServerHealth {
	    status: string;
	    service: string;
	    version: string;
	    server_public_key: string;
	    wg_port: number;
	    endpoint: string;
	    client_public_key: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerHealth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.service = source["service"];
	        this.version = source["version"];
	        this.server_public_key = source["server_public_key"];
	        this.wg_port = source["wg_port"];
	        this.endpoint = source["endpoint"];
	        this.client_public_key = source["client_public_key"];
	    }
	}
	export class StoredConfig {
	    endpoint: string;
	    nickname: string;
	
	    static createFrom(source: any = {}) {
	        return new StoredConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpoint = source["endpoint"];
	        this.nickname = source["nickname"];
	    }
	}

}

