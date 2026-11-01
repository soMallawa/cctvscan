export interface ScanConfig {
 targets:string[];port_mode:'camera'|'comprehensive'|'custom';ports:string;scan_mode:'connect'|'auto'|'syn';rate:number;retry:number;wait:number;threads:number;
 adapter:string;adapter_ip:string;router_ip:string;router_mac:string;route_target:string;timeout_seconds:number;host_timeout_seconds:number;host_workers:number;
 http_requests:number;http_per_host:number;http_rps:number;max_targets:number;proxy_version_id:string;credential_set_id:string;check_credentials:boolean;capture:boolean;onvif:boolean;debug:boolean
}
export type Policy = Record<string,number|boolean>
export interface Bootstrap {defaults:ScanConfig;policy:Policy;auth_mode:string;version:string;proxy_test_configured:boolean}
export interface Job {id:string;name:string;state:string;phase:string;config:ScanConfig;priority:number;created_at:string;started_at:string|null;finished_at:string|null;error:string;result_count:number;camera_count:number;stream_count:number;inconclusive_count:number;cancel_requested:boolean;attempt:number;retry_of:string|null}
export interface Event {seq:string;job_id:string;kind:string;created_at?:string;at?:string;phase?:string;message?:string;data?:Record<string,unknown>}
export interface Runner {id:string;name:string;state:string;hostname:string;capabilities:string[];slots:number;active_jobs:number;last_seen_at:string;rss_bytes:number|null;cpu_percent:number|null;scanner_version:string}
export interface Proxy {id:string;version_id:string;name:string;scheme:string;host:string;port:number;authentication_present:boolean;created_at:string;last_test_at:string|null;last_test_ok:boolean|null;last_test_message:string}
export interface Profile {id:string;name:string;config:ScanConfig;created_at:string;updated_at:string}
export interface Schedule {misfire_policy?:string;overlap_policy?:string;id:string;name:string;cron:string;timezone:string;config:ScanConfig;enabled:boolean;next_run_at:string|null;last_run_at:string|null;created_at:string}
export interface CredentialSet {id:string;name:string;entry_count:number;created_at:string}
export interface Result {id:string;job_id:string;target:string;brand:string;state:string;result:Record<string,unknown>}
export interface Page<T> {items:T[];total:number}
export interface Preview {normalized_targets:string[];requested_address_upper_bound:number;unique_ports:number;effective_ports:string;config:ScanConfig}
export interface Overview {counts:Record<string,number>;series:{day:string;jobs:number;completed:number;failed:number}[];result_count:number;camera_count:number;stream_count:number;recent_jobs:Job[];runners:Runner[]}
export const terminalStates = new Set(['completed','inconclusive','failed','canceled','timed_out','orphaned'])
