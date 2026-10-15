from __future__ import annotations
import logging
logger=logging.getLogger(__name__)
import asyncio
import json
import os
import re
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from datetime import datetime, timezone
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.performance import is_performance_query, load_snapshot, render_card, read_cached_card, write_cached_card, next_refresh_delay, REFRESH_INTERVAL
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_client import ReadOnlyReportClient
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_report_source import load_detection_report_snapshot as load_report_snapshot
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_delivery import ReportDelivery, load_report_config, DefiniteSendFailure, content_digest, report_is_usable
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_commands import parse_report_query
XINGQIAO_PUBLIC_BASE = 'https://api.xingqiaolab.top'
LOGIN_ENV_FILES = (Path('/opt/qqbot-native/secrets.env'), Path('/opt/qqbot/secrets.env'), Path('/opt/qqbot/.env'))
UA = 'Mozilla/5.0 (compatible; XingqiaoOpsBot/1.0; +https://api.xingqiaolab.top/)'
IMAGE_GUIDE = '宝宝，小星这边看不了图片呀～你把报错原文、状态码或者关键日志复制粘贴过来，我帮你看是哪类问题、怎么处理。'
FRIENDLY_ERROR = '宝，我暂时无法提供服务，管理员正在处理～'
_ERROR_MARKERS = ('LLM 响应错误', 'LLM 请求失败', 'All chat models failed', 'Error occurred while processing agent', 'Error occurred during AI execution', 'InternalServerError', 'local_capacity_exhausted', 'APIConnectionError', 'APITimeoutError', 'Error code: 5', '未找到指定的提供商')

def is_tech_error(text: str) -> bool:
    return bool(text) and any((marker in text for marker in _ERROR_MARKERS))
BOT_AT_NAMES = ('小星（24小时在线）', '小星(24小时在线)', '小星（24 小时在线）', '小星(24 小时在线)', '小星（24 小时主打陪伴）', '小星(24 小时主打陪伴)', '小星')
GROUP_KEEP = ('name', 'description', 'status', 'rate_multiplier')
_SECRET_RE = re.compile('(?i)(?<![A-Za-z0-9])(?:sk-[A-Za-z0-9_-]{16,}|sk-or-[A-Za-z0-9_-]{16,}|sk-ant-[A-Za-z0-9_-]{16,})')
_MD_LINK = re.compile('\\[([^\\]]+)\\]\\(([^)]+)\\)')
_MD_BOLD = re.compile('(\\*\\*|__)(.+?)\\1', re.S)
_MD_ITALIC = re.compile('(?<!\\*)\\*(?!\\*)([^*\\n]+)\\*(?!\\*)')
_MD_HEADING = re.compile('^#{1,6}\\s*', re.M)
_MD_STARS = re.compile('\\*{2,}')
_MD_CODE = re.compile('`+')

def redact_secrets(text: str) -> str:
    if not text:
        return text
    return _SECRET_RE.sub('[密钥已隐藏]', text)

def strip_markdown(text: str) -> str:
    if not text or ('*' not in text and '_' not in text and ('`' not in text) and ('#' not in text) and ('[' not in text)):
        return text
    text = _MD_LINK.sub('\\1 \\2', text)
    text = _MD_BOLD.sub('\\2', text)
    text = _MD_ITALIC.sub('\\1', text)
    text = _MD_CODE.sub('', text)
    text = _MD_HEADING.sub('', text)
    text = _MD_STARS.sub('', text)
    return text

def _http_json(url: str, headers: dict | None=None, timeout: int=15, method: str='GET', data=None) -> dict:
    req_headers = {'User-Agent': UA, 'Accept': 'application/json'}
    if headers:
        req_headers.update(headers)
    body = None
    if data is not None:
        body = json.dumps(data).encode('utf-8')
        req_headers['Content-Type'] = 'application/json'
    request = urllib.request.Request(url, data=body, headers=req_headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read(400000)
            try:
                return {'ok': True, 'status': response.status, 'data': json.loads(raw.decode('utf-8', 'replace'))}
            except json.JSONDecodeError:
                return {'ok': False, 'status': response.status, 'error': 'non-json response'}
    except urllib.error.HTTPError as exc:
        raw_body = ''
        try:
            raw_body = exc.read(2000).decode('utf-8', 'replace')
        except Exception:
            raw_body = str(exc)
        try:
            parsed = json.loads(raw_body)
        except Exception:
            parsed = raw_body
        return {'ok': False, 'status': exc.code, 'error': parsed}
    except Exception as exc:
        return {'ok': False, 'status': 0, 'error': str(exc)}

def _payload_data(result: dict):
    if not result.get('ok'):
        return None
    body = result.get('data')
    if isinstance(body, dict) and 'data' in body:
        return body.get('data')
    return body

def _read_env_file(path: Path) -> dict[str, str]:
    env: dict[str, str] = {}
    try:
        text = path.read_text(encoding='utf-8')
    except Exception:
        return env
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith('#') or '=' not in line:
            continue
        (key, value) = line.split('=', 1)
        env[key.strip()] = value.strip().strip('"').strip("'")
    return env

def _load_env_value(name: str) -> str:
    value = (os.environ.get(name) or '').strip()
    if value:
        return value
    for path in LOGIN_ENV_FILES:
        value = (_read_env_file(path).get(name) or '').strip()
        if value:
            return value
    return ''

def _load_admin_api_key() -> str:
    return _load_env_value('XINGQIAO_ADMIN_API_KEY')

def _load_login() -> tuple[str, str]:
    email = _load_env_value('XINGQIAO_EMAIL')
    password = _load_env_value('XINGQIAO_PASSWORD')
    return (email, password)

def _pick_group(item: dict) -> dict:
    out = {key: item.get(key) for key in GROUP_KEEP if key in item}
    return out
_REQUEST_ID_RE = re.compile('^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$')
_USER_ERROR_TYPES = {'rate_limit_error', 'billing_error', 'subscription_error', 'invalid_request_error', 'cyber_policy', 'authentication_error'}

def _list_items(payload) -> list:
    if isinstance(payload, list):
        return [item for item in payload if isinstance(item, dict)]
    if not isinstance(payload, dict):
        return []
    for key in ('items', 'errors', 'list'):
        value = payload.get(key)
        if isinstance(value, list):
            return [item for item in value if isinstance(item, dict)]
    nested = payload.get('data')
    if nested is not None and nested is not payload:
        return _list_items(nested)
    return []

def _normalize_request_id(value: str) -> str:
    text = (value or '').strip().strip('"\'` ,;。')
    text = re.sub('^(request[_-]?id|x-request-id|请求id)\\s*[:=：]\\s*', '', text, flags=re.I)
    return text.strip()

def _valid_request_id(value: str) -> bool:
    return bool(_REQUEST_ID_RE.match(value or ''))

def _classify_request_error(item: dict) -> str:
    owner = str(item.get('error_owner') or item.get('owner') or '').strip().lower()
    phase = str(item.get('phase') or '').strip().lower()
    err_type = str(item.get('type') or item.get('error_type') or '').strip().lower()
    if owner == 'client' or phase == 'auth' or err_type in _USER_ERROR_TYPES:
        return 'user'
    if owner == 'provider' or phase in {'upstream', 'network', 'account_auth'}:
        return 'upstream'
    if owner == 'platform' or phase in {'internal', 'routing'}:
        return 'internal'
    return 'unknown'

def _user_reason_for_error(item: dict, outcome: str) -> tuple[str, str]:
    status = item.get('status_code') or item.get('status')
    err_type = str(item.get('type') or '').strip().lower()
    if outcome == 'user':
        if err_type in {'billing_error', 'subscription_error'} or status in {402}:
            return ('额度、余额或订阅不可用', '去星桥控制台看余额和订阅，不够就充值后再试')
        if err_type == 'rate_limit_error' or status == 429:
            return ('请求太快，触发了限流', '先降并发、隔几秒再试')
        if err_type in {'authentication_error'} or status in {401, 403}:
            return ('密钥无效或当前分组不让用这个模型', '回控制台核对密钥和分组模型')
        if err_type == 'invalid_request_error' or status in {400, 404, 422}:
            return ('请求参数或模型名不对', '核对模型 ID、请求格式后再发')
        if err_type == 'cyber_policy':
            return ('请求不符合当前使用规则', '换可用模型，或按当前分组规则改请求')
        return ('这次是请求本身的问题', '把报错原文和模型名再核对一遍，不行就找群里管理员')
    if outcome == 'upstream':
        if status in {429, 529}:
            return ('上游现在比较忙', '稍后再试；一直不行把这个 request id 发给群里管理员')
        return ('上游这次没打通', '稍后再试；一直不行把这个 request id 发给群里管理员')
    return ('暂时还不知道具体原因', '把这个 request id 发给群里管理员帮看')

def _match_groups(groups: list[dict], query: str) -> list[dict]:
    want = (query or '').strip().lower()
    if not want:
        return []
    aliases = {'pro': 'pro', 'plus': 'plus', '特惠': '特惠', '生图': '生图', 'grok': 'grok', 'claude': 'claude', 'deepseek': 'deepseek', '测试': '测试'}
    needles = [want]
    for (alias, needle) in aliases.items():
        if alias in want:
            needles.append(needle)
    matched = []
    for group in groups:
        name = str(group.get('name') or '')
        blob = f"{name} {group.get('description') or ''}".lower()
        if any((needle.lower() in blob for needle in needles)):
            matched.append(group)
    return matched

class Business:

    def __init__(self):
        self._token = ''
        self._token_exp = 0.0
        self._role = ''

    def _login(self, force: bool=False) -> dict:
        if _load_admin_api_key():
            self._role = 'admin'
            return {'ok': True, 'token': '', 'role': 'admin', 'auth': 'admin_api_key'}
        if not force and self._token and (time.time() < self._token_exp - 120):
            return {'ok': True, 'token': self._token, 'role': self._role}
        (email, password) = _load_login()
        if not email or not password:
            return {'ok': False, 'error': 'missing_login'}
        result = _http_json(f'{XINGQIAO_PUBLIC_BASE}/api/v1/auth/login', method='POST', data={'email': email, 'password': password}, timeout=20)
        data = _payload_data(result)
        token = ''
        role = ''
        expires_in = 3600
        if isinstance(data, dict):
            token = str(data.get('access_token') or '')
            expires_in = int(data.get('expires_in') or 3600)
            user = data.get('user') if isinstance(data.get('user'), dict) else {}
            role = str(user.get('role') or '')
        if not token:
            logger.warning('xingqiao login failed status=%s', result.get('status'))
            self._token = ''
            self._role = ''
            return {'ok': False, 'error': 'login_failed', 'status': result.get('status')}
        self._token = token
        self._token_exp = time.time() + max(expires_in, 60)
        self._role = role
        return {'ok': True, 'token': token, 'role': role, 'auth': 'password'}

    def _auth_headers(self, force: bool=False) -> dict | None:
        admin_key = _load_admin_api_key()
        if admin_key:
            return {'X-API-Key': admin_key}
        login = self._login(force=force)
        if not login.get('ok'):
            return None
        return {'Authorization': f"Bearer {login['token']}"}

    def _auth_get(self, path: str) -> dict:
        headers = self._auth_headers()
        if not headers:
            return {'ok': False, 'status': 401, 'error': 'login_failed'}
        result = _http_json(f'{XINGQIAO_PUBLIC_BASE}{path}', headers=headers)
        if result.get('status') == 401:
            headers = self._auth_headers(force=True)
            if not headers:
                return result
            result = _http_json(f'{XINGQIAO_PUBLIC_BASE}{path}', headers=headers)
        return result

    def _group_public_view(self, group: dict) -> dict:
        return {'name': group.get('name'), 'status': group.get('status'), 'description': group.get('description'), 'rate_multiplier': group.get('rate_multiplier')}

    def _lookup_request(self, request_id: str) -> dict:
        rid = _normalize_request_id(request_id)
        if not _valid_request_id(rid):
            return {'found': False, 'outcome': 'unknown', 'user_facing_reason': '这个 request id 看起来不完整', 'user_action': '把完整的 request id 再发一次，或咨询群里管理员'}
        query = urllib.parse.urlencode({'request_id': rid, 'time_range': '30d', 'page': 1, 'page_size': 10})
        q_search = urllib.parse.urlencode({'q': rid, 'time_range': '30d', 'view': 'all', 'page': 1, 'page_size': 10})
        ops = _payload_data(self._auth_get(f'/api/v1/admin/ops/requests?{query}'))
        errors = _payload_data(self._auth_get(f'/api/v1/admin/ops/errors?{q_search}'))
        usage = _payload_data(self._auth_get(f'/api/v1/admin/usage?{query}'))
        ops_items = _list_items(ops)
        error_items = _list_items(errors)
        usage_items = _list_items(usage)
        error_item = next((item for item in error_items if str(item.get('request_id') or item.get('client_request_id') or '') == rid), error_items[0] if error_items else None)
        ops_item = next((item for item in ops_items if str(item.get('request_id') or '') == rid), ops_items[0] if ops_items else None)
        usage_item = next((item for item in usage_items if str(item.get('request_id') or '') == rid), usage_items[0] if usage_items else None)
        if error_item is None and ops_item and ops_item.get('error_id'):
            detail = _payload_data(self._auth_get(f"/api/v1/admin/ops/errors/{ops_item['error_id']}"))
            if isinstance(detail, dict):
                error_item = detail
        if error_item is None and ops_item and (str(ops_item.get('kind') or '') == 'error'):
            error_item = ops_item
        if error_item is not None:
            outcome = _classify_request_error(error_item)
            if outcome == 'unknown' and str(ops_item.get('kind') if ops_item else '') == 'error':
                outcome = _classify_request_error(ops_item)
            (reason, action) = _user_reason_for_error(error_item, outcome)
            status = error_item.get('status_code') or (ops_item or {}).get('status_code')
            model = error_item.get('requested_model') or error_item.get('model') or (ops_item or {}).get('model')
            return {'found': True, 'request_id': rid, 'outcome': outcome, 'http_status': status if isinstance(status, int) else None, 'requested_model': str(model or '') or None, 'user_facing_reason': reason, 'user_action': action}
        if usage_item is not None or (ops_item and str(ops_item.get('kind') or '') == 'success'):
            model = (usage_item or {}).get('model') or (ops_item or {}).get('model')
            return {'found': True, 'request_id': rid, 'outcome': 'success', 'requested_model': str(model or '') or None, 'user_facing_reason': '这条请求在星桥这边是打成功的', 'user_action': '若你那边超时或没收到，多半是客户端等太久或网络断开，可以再试一次'}
        return {'found': False, 'request_id': rid, 'outcome': 'unknown', 'user_facing_reason': '暂时没查到这条请求', 'user_action': '确认 request id 完整后再发，或把这个 id 发给群里管理员'}

    async def xingqiao_public_config(self, event, query: str) -> str:
        """读取星桥当前公开设置和用户侧分组信息（名称、简介、状态、倍率）。

        Args:
            query(string): 用户想查的内容，例如 分组、GPT-Pro、今天卡不卡、公开配置
        """
        del event
        want = (query or '').strip() or '公开配置'
        public_settings = _http_json(f'{XINGQIAO_PUBLIC_BASE}/api/v1/settings/public')
        public_data = _payload_data(public_settings)
        public_view = {}
        if isinstance(public_data, dict):
            public_view = {'site_name': public_data.get('site_name'), 'api_base_url': public_data.get('api_base_url'), 'doc_url': public_data.get('doc_url'), 'registration_enabled': public_data.get('registration_enabled'), 'email_verify_enabled': public_data.get('email_verify_enabled'), 'registration_email_suffix_whitelist': public_data.get('registration_email_suffix_whitelist'), 'invitation_code_enabled': public_data.get('invitation_code_enabled'), 'purchase_subscription_url': public_data.get('purchase_subscription_url')}
        login = self._login()
        groups: list[dict] = []
        if login.get('ok'):
            admin_groups = _payload_data(self._auth_get('/api/v1/admin/groups?page=1&page_size=50'))
            source = []
            if isinstance(admin_groups, dict) and isinstance(admin_groups.get('items'), list):
                source = admin_groups['items']
            elif login.get('auth') != 'admin_api_key':
                available = _payload_data(self._auth_get('/api/v1/groups/available'))
                if isinstance(available, list):
                    source = available
            for item in source:
                if isinstance(item, dict):
                    groups.append(_pick_group(item))
        matched = [self._group_public_view(group) for group in _match_groups(groups, want)[:4]]
        summary = {'query': want, 'site': '星桥 AI Link', 'api_base': 'https://api.xingqiaolab.top', 'openai_compatible_base': 'https://api.xingqiaolab.top/v1', 'docs': 'https://api.xingqiaolab.top/docs/', 'live_login_ok': bool(login.get('ok')), 'public_settings': public_view, 'groups': groups, 'matched_groups': matched, 'reply_rules': ['只用 groups / matched_groups 里的名称、简介、状态、倍率、公开档位口语回答', '今天稳不稳只说能用/挺稳/有点抖，不要报号数、线路数、成功率、请求量', '有人问号池或监控数字：请对方咨询群里管理员，不要说自己没权限或看不到报表', '不要说登录、知识库、工具、翻配置', '不要输出账号名、供应商、内部地址、密钥；用户发来的密钥不要复读', '不要使用 Markdown 星号', '用户没点名厂商时过载只说上游']}
        if not login.get('ok'):
            summary['live_error'] = '未能读取实时分组，不要编造分组名，请用户咨询群里管理员或打开站点前台'
        logger.info('xingqiao_public_config groups=%s matched=%s login=%s', len(groups), len(matched), bool(login.get('ok')))
        return json.dumps(summary, ensure_ascii=False)[:12000]

    async def xingqiao_lookup_request(self, event, request_id: str) -> str:
        """按用户提供的 request id 查看这次请求为什么失败。只返回可对用户说的结论。

        Args:
            request_id(string): 用户消息里的 request id / request_id / 请求 ID
        """
        del event
        login = self._login()
        if not login.get('ok'):
            view = {'found': False, 'outcome': 'unknown', 'user_facing_reason': '暂时没查到这条请求', 'user_action': '把这个 request id 发给群里管理员'}
        else:
            view = self._lookup_request(request_id)
        view['reply_rules'] = ['只根据 outcome / user_facing_reason / user_action 用口语回答', 'outcome=user：用人话讲密钥、额度、参数或限流，给 1 到 3 条操作', 'outcome=upstream：只说上游忙或没打通，让他稍后重试，不要点名厂商', 'outcome=internal、unknown 或 found=false：说暂时还不知道具体原因，请把这个 request id 发给群里管理员', '不要说查服务器、后台、日志、登录、工具', '不要输出账号名、供应商、内部地址、堆栈、号池、IP、邮箱、密钥', '不要使用 Markdown 星号']
        logger.info('xingqiao_lookup_request found=%s outcome=%s', view.get('found'), view.get('outcome'))
        return json.dumps(view, ensure_ascii=False)[:4000]
