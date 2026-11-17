from datetime import datetime, timezone
import getpass
import json
import secrets
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, quote, urlencode, urlsplit
import re
from .net import APIError, request_json, secret_read, secret_write
from .state import digest

API = "https://api.tumblr.com"


def blog_name(value):
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*\.tumblr\.com", value):
        raise ValueError("Configure the exact lowercase blog hostname, e.g. your-blog.tumblr.com.")
    return value


def connect(config, port=8766):
    client_id = input("Tumblr OAuth consumer key/client ID: ").strip()
    client_secret = getpass.getpass("Tumblr OAuth consumer secret (hidden): ").strip()
    if not client_id or not client_secret:
        raise ValueError("Both app credentials are required.")
    redirect = f"http://localhost:{port}/callback"
    state = secrets.token_urlsafe(32)
    result = {}

    class Callback(BaseHTTPRequestHandler):
        def do_GET(self):
            parsed = urlsplit(self.path)
            values = parse_qs(parsed.query)
            valid = parsed.path == "/callback" and secrets.compare_digest(values.get("state", [""])[0], state)
            if not valid:
                self.send_response(400)
                self.end_headers()
                self.wfile.write(b"Invalid callback.")
                return
            result["code"] = values.get("code", [""])[0]
            result["error"] = values.get("error", [""])[0]
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"You may close this tab and return to GenTalent.")

        def log_message(self, *args):
            pass

    query = {"client_id": client_id, "response_type": "code",
             "scope": "basic write offline_access", "state": state, "redirect_uri": redirect}
    with HTTPServer(("127.0.0.1", port), Callback) as server:
        server.timeout = 1
        print("Register this exact OAuth2 redirect URL in your Tumblr app:", redirect)
        print("Open this URL yourself and approve only the intended account:")
        print("https://www.tumblr.com/oauth2/authorize?" + urlencode(query))
        end = time.monotonic() + 300
        while not result and time.monotonic() < end:
            server.handle_request()
    if result.get("error") or not result.get("code"):
        raise ValueError("Authorization was declined or timed out. No credentials were saved.")
    token = request_json("POST", API + "/v2/oauth2/token", form={
        "grant_type": "authorization_code", "code": result["code"], "client_id": client_id,
        "client_secret": client_secret, "redirect_uri": redirect})
    if not token.get("access_token"):
        raise APIError("Tumblr did not provide an access token.")
    token.update(client_id=client_id, client_secret=client_secret,
                 expires_at=time.time() + int(token.get("expires_in", 3600)))
    secret_write(config.path("tumblr_credentials"), json.dumps(token))
    print("Tumblr authorization saved privately in WSL. No post was created.")


class Tumblr:
    def __init__(self, config):
        self.path = config.path("tumblr_credentials")
        self.blog = blog_name(config.data["tumblr_blog"])
        self.credentials = json.loads(secret_read(self.path))

    def token(self):
        c = self.credentials
        if c.get("expires_at", 0) <= time.time() + 60:
            if not c.get("refresh_token"):
                raise ValueError("Tumblr authorization expired; run connect-tumblr again.")
            new = request_json("POST", API + "/v2/oauth2/token", form={
                "grant_type": "refresh_token", "refresh_token": c["refresh_token"],
                "client_id": c["client_id"], "client_secret": c["client_secret"]})
            if not new.get("access_token"):
                raise APIError("Tumblr token refresh failed.")
            c.update(new)
            c["expires_at"] = time.time() + int(new.get("expires_in", 3600))
            secret_write(self.path, json.dumps(c))
        return c["access_token"]

    def request(self, method, suffix, body=None):
        result = request_json(method, API + suffix, token=self.token(), body=body)
        if result.get("meta", {}).get("status", 200) >= 400:
            raise APIError("Tumblr rejected the operation; inspect authorization and blog access.")
        return result.get("response", {})

    def verify_owner(self):
        result = self.request("GET", "/v2/user/info")
        urls = {urlsplit(b.get("url", "")).hostname for b in result.get("user", {}).get("blogs", [])}
        if self.blog not in urls:
            raise ValueError("The configured blog is not in this account's owned/managed blogs.")

    def retrieve(self, post_id):
        return self.request("GET", "/v2/blog/" + quote(self.blog, safe="") + "/posts/" + quote(post_id, safe=""))

    def write(self, payload, post_id=None):
        path = "/v2/blog/" + quote(self.blog, safe="") + "/posts"
        if post_id:
            return self.request("PUT", path + "/" + quote(post_id, safe=""), payload)
        return self.request("POST", path, payload)


def publish(store, client):
    row = store.current()
    payload = store.payload()
    version = digest(payload)
    if row["approved_hash"] != version:
        raise ValueError("Approve this exact public preview in the group with !approve-public first.")
    if row["publish_state"] in ("sending", "uncertain"):
        raise ValueError("The previous publication outcome is uncertain. Reconcile its post ID first.")
    if row["published_hash"] == version:
        return row["public_url"]
    client.verify_owner()
    with store.db:
        store.db.execute("UPDATE cases SET publish_state='sending' WHERE id=?", (row["id"],))
    try:
        result = client.write(payload, row["post_id"])
        post_id = str(result.get("id_string") or result.get("id") or "")
        if not post_id:
            raise APIError("Tumblr response did not include a post ID.")
        # Persist an obtained ID before verification, so recovery never needs a second create.
        with store.db:
            store.db.execute("UPDATE cases SET post_id=? WHERE id=?", (post_id, row["id"]))
        post = client.retrieve(post_id)
        if post.get("state") != "published" or post.get("content") != payload["content"]:
            raise APIError("The resulting public post has not been verified.")
    except Exception:
        with store.db:
            store.db.execute("UPDATE cases SET publish_state='uncertain' WHERE id=?", (row["id"],))
        raise APIError("Tumblr outcome is uncertain. Check the exact blog; do not create another post.") from None
    return finish_publication(store, client.blog, post_id, version)


def finish_publication(store, blog, post_id, version):
    row = store.current()
    url = f"https://{blog}/post/{post_id}"
    with store.db:
        store.db.execute("UPDATE cases SET post_id=?,published_hash=?,public_url=?,publish_state='published' WHERE id=?",
                         (post_id, version, url, row["id"]))
        store.queue(f"{row['name']} · public record: {url}", "public-link")
    return url


def reconcile(store, client, post_id):
    row = store.current()
    if row["publish_state"] != "uncertain":
        raise ValueError("Only an uncertain publication requires reconciliation.")
    client.verify_owner()
    post = client.retrieve(post_id)
    payload = store.payload()
    if post.get("state") != "published" or post.get("content") != payload["content"]:
        raise ValueError("That post does not match the currently approved public content.")
    if row["approved_hash"] != digest(payload):
        raise ValueError("Current content approval is missing.")
    return finish_publication(store, client.blog, post_id, digest(payload))
