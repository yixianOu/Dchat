export namespace chat {
	
	export class User {
	    id: string;
	    nickname: string;
	
	    static createFrom(source: any = {}) {
	        return new User(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.nickname = source["nickname"];
	    }
	}

}

export namespace main {
	
	export class CreateGroupResult {
	    gid: string;
	    groupKey: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateGroupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gid = source["gid"];
	        this.groupKey = source["groupKey"];
	    }
	}

}

export namespace storage {
	
	export class StoredConversation {
	    id: string;
	    type: string;
	    last_message_at: time.Time;
	    created_at: time.Time;
	
	    static createFrom(source: any = {}) {
	        return new StoredConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.last_message_at = this.convertValues(source["last_message_at"], time.Time);
	        this.created_at = this.convertValues(source["created_at"], time.Time);
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
	export class StoredMessage {
	    id: string;
	    conversation_id: string;
	    sender_id: string;
	    sender_nickname: string;
	    content: string;
	    timestamp: time.Time;
	    is_read: boolean;
	    is_group: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StoredMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.sender_id = source["sender_id"];
	        this.sender_nickname = source["sender_nickname"];
	        this.content = source["content"];
	        this.timestamp = this.convertValues(source["timestamp"], time.Time);
	        this.is_read = source["is_read"];
	        this.is_group = source["is_group"];
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

export namespace time {
	
	export class Time {
	
	
	    static createFrom(source: any = {}) {
	        return new Time(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

