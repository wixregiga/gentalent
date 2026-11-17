import html
import json
import mimetypes
import re
import os
from pathlib import Path
import uuid
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlencode
from urllib.request import Request, HTTPRedirectHandler, ProxyHandler, build_opener


LINE_TAGS = re.compile(r"</?(?:br|p|div|li|ul|ol|blockquote|h[1-6])\b[^>]*>", re.IGNORECASE)


def plain_text(value):
    """The text people see. Beeper stores sent Markdown as HTML (blocks as <p>, hard breaks
    as <br>; observed in doc/beeper-api.md item 7). Line-level tags become spaces, inline tags
    such as <em> vanish, and entities are decoded."""
    return html.unescape(re.sub(r"<[^>]+>", "", LINE_TAGS.sub(" ", value or "")))


class APIError(Exception):
    def __init__(self, message, status_code=None):
        super().__init__(message)
        self.status_code = status_code


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def secret_read(path):
    path = Path(path).expanduser()
    if path.stat().st_mode & 0o077:
        raise ValueError(f"Secret file must be private: chmod 600 {path}")
    return path.read_text().strip()


def secret_write(path, value):
    path = Path(path).expanduser()
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temp = path.with_name(path.name + ".new")
    descriptor = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w") as stream:
        stream.write(value)
    os.chmod(temp, 0o600)
    temp.replace(path)


def request_json(method, url, token=None, body=None, form=None, local=False):
    headers = {"User-Agent": "GenTalent/0.1", "Accept": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode()
        headers["Content-Type"] = "application/json"
    if form is not None:
        data = urlencode(form).encode()
        headers["Content-Type"] = "application/x-www-form-urlencoded"
    handlers = [NoRedirect()]
    if local:
        handlers.append(ProxyHandler({}))
    opener = build_opener(*handlers)
    try:
        with opener.open(Request(url, data=data, headers=headers, method=method), timeout=15) as response:
            return json.load(response)
    except HTTPError as error:
        raise APIError(
            f"API returned HTTP {error.code}; no automatic write retry was made.",
            status_code=error.code,
        ) from None
    except (URLError, TimeoutError, OSError, ValueError):
        raise APIError("API connection or JSON response failed; a write outcome may be uncertain.") from None


def request_multipart(url, token, file_path, field_name="file", mime_type="image/jpeg"):
    path = Path(file_path)
    if not path.is_file():
        raise APIError(f"Attachment file does not exist: {path}")
    guessed = mimetypes.guess_type(path.name)[0]
    ctype = guessed or mime_type
    safe_name = path.name.replace('"', "").replace("\r", "").replace("\n", "")
    boundary = "----GenTalentBoundary" + uuid.uuid4().hex
    payload = path.read_bytes()
    body = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="{field_name}"; filename="{safe_name}"\r\n'
        f"Content-Type: {ctype}\r\n\r\n"
    ).encode("utf-8") + payload + f"\r\n--{boundary}--\r\n".encode("utf-8")
    headers = {
        "User-Agent": "GenTalent/0.1",
        "Accept": "application/json",
        "Content-Type": f"multipart/form-data; boundary={boundary}",
    }
    if token:
        headers["Authorization"] = "Bearer " + token
    opener = build_opener(NoRedirect(), ProxyHandler({}))
    try:
        with opener.open(Request(url, data=body, headers=headers, method="POST"), timeout=30) as response:
            return json.load(response)
    except HTTPError as error:
        raise APIError(
            f"Asset upload returned HTTP {error.code}; no automatic write retry was made.",
            status_code=error.code,
        ) from None
    except (URLError, TimeoutError, OSError, ValueError):
        raise APIError("Asset upload connection or JSON response failed.") from None


class Beeper:
    def __init__(self, config, discovery=False):
        config.validate_base() if discovery else config.validate()
        self.config = config
        self.base = config.data["beeper_url"].rstrip("/")
        self.token = secret_read(config.path("token_file"))

    def request(self, method, path, body=None, query=None):
        url = self.base + path
        if query:
            url += "?" + urlencode(query, doseq=True)
        return request_json(method, url, token=self.token, body=body, local=True)

    @property
    def chat_path(self):
        return "/v1/chats/" + quote(self.config.data["chat_id"], safe="")

    def verify_chat(self):
        chat = self.request("GET", self.chat_path, query={"maxParticipantCount": -1})
        d = self.config.data
        if chat.get("id") != d["chat_id"] or chat.get("accountID") != d["account_id"] or chat.get("type") != "group":
            raise APIError("Chat/account/type differs from the configured group; stopped.")
        participants = chat.get("participants", {})
        if participants.get("hasMore"):
            raise APIError("The complete group roster could not be verified.")
        ids = {m["id"] for m in participants.get("items", [])}
        expected = set(self.config.roster)
        if ids not in (expected, expected - {d["owner_id"]}):
            raise APIError("The current group roster differs from the confirmed three-person roster.")
        return chat

    def messages(self, cursor=None):
        query = {"cursor": cursor, "direction": "after"} if cursor else None
        return self.request("GET", self.chat_path + "/messages", query=query)

    def upload_asset(self, file_path):
        result = request_multipart(self.base + "/v1/assets/upload", self.token, file_path)
        if result.get("error") or not result.get("uploadID"):
            raise APIError(f"Asset upload failed: {result.get('error') or 'missing uploadID'}")
        return result["uploadID"]

    def send(self, text, attachment_path=None):
        if not text and not attachment_path:
            raise ValueError("A message needs text, an attachment, or both.")
        self.verify_chat()
        body = {"text": text} if text else {}
        if attachment_path:
            path = Path(attachment_path)
            upload_id = self.upload_asset(path)
            mime = mimetypes.guess_type(path.name)[0] or "image/jpeg"
            body["attachment"] = {
                "uploadID": upload_id,
                "type": "gif" if mime == "image/gif" else "image",
                "fileName": path.name,
                "mimeType": mime,
            }
        result = self.request("POST", self.chat_path + "/messages", body)
        if result.get("chatID") != self.config.data["chat_id"] or not result.get("pendingMessageID"):
            raise APIError("Send response did not confirm the configured underlying chat and pending ID.")
        return result["pendingMessageID"]

    def retrieve(self, message_id):
        result = self.request("GET", self.chat_path + "/messages/" + quote(message_id, safe=""))
        if result.get("chatID") != self.config.data["chat_id"] or result.get("accountID") != self.config.data["account_id"]:
            raise APIError("Retrieved message has the wrong chat/account.")
        return result
