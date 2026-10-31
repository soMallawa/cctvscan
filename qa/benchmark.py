#!/usr/bin/env python3
"""Pure CPU/allocation benchmarks in a disposable source copy; never scans."""
import os,pathlib,shutil,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='cctv-bench-') as d:
 d=pathlib.Path(d)
 for name in ('go.mod','go.sum'):shutil.copy2(root/name,d/name)
 shutil.copytree(root/'internal',d/'internal')
 (d/'internal/fingerprint/closure_benchmark_test.go').write_text('''package fingerprint
import("fmt";"testing")
func BenchmarkCVELookupAfterBodyCache(b *testing.B){for _,n:=range []int{1000,10000}{b.Run(fmt.Sprint(n),func(b *testing.B){ClearCache();defer ClearCache();for i:=0;i<n;i++{OptimizedDetect("nginx",fmt.Sprintf("ordinary page %d",i),"")};b.ResetTimer();for i:=0;i<b.N;i++{OptimizedCVEsForBrand("Hikvision")};b.StopTimer();entries,_:=GetCacheStats();b.ReportMetric(float64(entries),"retained_entries")})}}
''')
 (d/'internal/targets/closure_benchmark_test.go').write_text('''package targets
import("context";"testing")
func BenchmarkCompactTargets(b *testing.B){for _,prefix:=range []string{"198.18.0.0/16","0.0.0.0/0"}{b.Run(prefix,func(b *testing.B){for i:=0;i<b.N;i++{r,e:=Normalize(context.Background(),[]string{prefix});if e!=nil||len(r)!=1{b.Fatal(r,e)}}})}}
''')
 subprocess.run(['go','test','./internal/fingerprint','./internal/targets','-run','^$','-bench','CVELookup|CompactTargets','-benchmem','-benchtime=200ms','-count=2'],cwd=d,check=True,env=os.environ.copy())
