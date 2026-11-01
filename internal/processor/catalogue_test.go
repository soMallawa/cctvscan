package processor
import("context";"net";"net/http";"net/http/httptest";"strconv";"testing";"github.com/postfix/cctvscan/internal/catalogue")
func TestProcessedCameraCarriesSourceBoundCatalogueCandidates(t *testing.T){
 c,e:=catalogue.Default();if e!=nil{t.Fatal(e)};m:=c.Search("DS-2CD","hikvision","",1,0).Items[0]
 s:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.URL.Path!="/"{http.NotFound(w,r);return};w.Header().Set("Server","Hikvision");w.Write([]byte("<title>Hikvision IP Camera</title>Model: "+m.Model))}));defer s.Close()
 _,port,_:=net.SplitHostPort(s.Listener.Addr().String());pn,_:=strconv.Atoi(port);p:=NewOptimizedProcessor(false,"",t.TempDir());r:=p.processHost(context.Background(),"127.0.0.1",[]int{pn})
 if r.Catalogue==nil || r.Catalogue.State!="model_candidate" || len(r.Catalogue.Candidates)==0 || r.Catalogue.Revision!=c.Manifest.Revision{t.Fatal("processed evidence did not reach the actual catalogue")}
}
