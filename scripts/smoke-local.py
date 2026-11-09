"""Isolated HTTPS/browser smoke test. Requires Python cryptography and Node playwright.
Uses a simulated 3x-ui API, temporary database and self-signed test certificate.
"""
import datetime, http.server, ipaddress, json, os, pathlib, socket, ssl, subprocess, tempfile, threading, time, urllib.request
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

root = pathlib.Path(__file__).resolve().parents[1]
EXPIRY = int((time.time()+30*86400)*1000)
class Panel(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_GET(self):
        expiry = EXPIRY
        record = dict(id=9,email='demo-client',subId='fixture-subscription',enable=True,expiryTime=expiry,totalGB=107374182400)
        detail = dict(client=record,inboundIds=[1])
        path=self.path.split('?')[0]
        if path.startswith('/subscription/'):
            self.send_response(200);self.send_header('Content-Type','text/plain');self.end_headers();self.wfile.write(b'fixture-subscription-content');return
        if self.headers.get('Authorization') != 'Bearer fixture-panel-token-for-test-only':
            self.send_response(401);self.end_headers();return
        if path.endswith('/list/paged'): obj=dict(items=[record],total=1)
        elif path.endswith('/clients/get'): obj=detail
        elif '/clients/get/subId/' in path: obj=[detail]
        elif '/clients/traffic/' in path: obj=dict(email='demo-client',enable=True,up=1073741824,down=5368709120,total=107374182400,expiryTime=expiry)
        elif '/clients/subLinks/' in path: obj=[f'http://127.0.0.1:{self.server.server_port}/subscription/fixture-subscription']
        else: self.send_response(404);self.end_headers();return
        self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(json.dumps(dict(success=True,obj=obj)).encode())

def main():
    (root/'.local').mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='smoke-',dir=root/'.local') as directory:
        d=pathlib.Path(directory);(d/'data').mkdir()
        with socket.socket() as s: s.bind(('127.0.0.1',0));port=s.getsockname()[1]
        panel=http.server.ThreadingHTTPServer(('127.0.0.1',0),Panel)
        threading.Thread(target=panel.serve_forever,daemon=True).start()
        origin=f'https://127.0.0.1:{port}'
        env=os.environ|dict(PORTAL_ORIGIN=origin,PORTAL_LISTEN=f'127.0.0.1:{port}',PORTAL_TLS_MODE='managed',PORTAL_DATA_DIR=str(d/'data'),PORTAL_KEY_FILE=str(d/'master.key'),PORTAL_WEB_DIR=str(root/'frontend/dist'))
        binary=str(root/('portal.exe' if os.name=='nt' else 'portal'))
        subprocess.run([binary,'init'],env=env,check=True,capture_output=True)
        key=ec.generate_private_key(ec.SECP256R1());subject=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'Portal local test')]);now=datetime.datetime.now(datetime.timezone.utc)
        cert=x509.CertificateBuilder().subject_name(subject).issuer_name(subject).public_key(key.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now-datetime.timedelta(minutes=5)).not_valid_after(now+datetime.timedelta(days=7)).add_extension(x509.SubjectAlternativeName([x509.IPAddress(ipaddress.ip_address('127.0.0.1'))]),False).sign(key,hashes.SHA256())
        (d/'cert.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM));(d/'key.pem').write_bytes(key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
        env|=dict(PORTAL_IMPORT_CERT=str(d/'cert.pem'),PORTAL_IMPORT_KEY=str(d/'key.pem'))
        subprocess.run([binary,'tls-import'],env=env,check=True,capture_output=True)
        log=(d/'service.log').open('w');process=subprocess.Popen([binary,'serve'],env=env,stdout=log,stderr=log)
        try:
            token=(d/'data/setup.token').read_text()
            ready=False
            for _ in range(80):
                try:
                    with urllib.request.urlopen(origin+'/install/'+token+'/api/bootstrap',context=ssl._create_unverified_context(),timeout=1) as r: ready=r.status==200
                    if ready: break
                except Exception: time.sleep(.15)
            assert ready,'local service did not become ready'
            env|=dict(SMOKE_ORIGIN=origin,SMOKE_SETUP_TOKEN=token,SMOKE_PANEL=f'http://127.0.0.1:{panel.server_port}',SMOKE_OUTPUT=str(root/'.local'))
            subprocess.run(['node',str(root/'scripts/browser-smoke.cjs')],env=env,check=True)
        finally:
            process.terminate();process.wait(timeout=15);log.close();(root/'.local/smoke-service-last.log').write_text((d/'service.log').read_text(encoding='utf-8',errors='replace'),encoding='utf-8');panel.shutdown();panel.server_close()
if __name__=='__main__': main()
