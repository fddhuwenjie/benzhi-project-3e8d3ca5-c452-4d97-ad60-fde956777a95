const state = { case: null, view: null, list: null };
const $ = (selector, root = document) => root.querySelector(selector);
const uid = () => crypto.randomUUID();
const statusText = { draft: '草拟', baseline_frozen: '基线已冻结', plan_approved: '方案已批准', in_progress: '施工中', remediation: '缺陷整改', awaiting_review: '待独立验收', sealed: '已封存' };
const esc = (value) => String(value ?? '').replace(/[&<>"']/g, (character) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character]));

async function api(path, options = {}) {
  const response = await fetch(path, { headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }, ...options });
  const data = await response.json();
  if (!response.ok) throw new Error(data.message || '请求失败');
  return data;
}
function notice(text, error = false) { const element = $('#notice'); element.textContent = text; element.className = error ? 'error' : ''; element.style.display = 'block'; setTimeout(() => { element.style.display = 'none'; }, 4000); }
function formData(form) { return Object.fromEntries(new FormData(form).entries()); }
function template(id) { return document.importNode($(id).content, true); }

async function loadOverview() {
  const data = formData($('#filters')); const params = new URLSearchParams({ page: '1', page_size: '50' });
  Object.entries(data).forEach(([key, value]) => { if (value) params.set(key, value); });
  state.list = await api(`/api/cases?${params}`); renderOverview();
}
function renderOverview() {
  const { items, stats } = state.list;
  $('#stats').innerHTML = `<span>案件 ${stats.total}</span><span>开放缺陷 ${stats.open_defects}</span><span>待独立验收 ${stats.awaiting_review}</span>`;
  const body = $('#case-list'); body.innerHTML = '';
  if (!items.length) { body.innerHTML = '<tr><td colspan="6" class="muted">没有符合条件的案件</td></tr>'; return; }
  items.forEach((item) => {
    const row = document.createElement('tr');
    row.innerHTML = `<td><strong>${esc(item.case_id)}</strong><small>${esc(item.site_code)} · ${esc(item.trench_coordinates)}</small></td><td>${esc(statusText[item.status] || item.status)}</td><td>${item.passed_layers}/${item.planned_layers}</td><td>${item.open_defects}</td><td>${esc(item.last_event_summary || '无')}</td><td><button type="button">打开</button></td>`;
    $('button', row).onclick = () => loadCase(item.case_id).catch((error) => notice(error.message, true)); body.append(row);
  });
}
async function loadCase(id) { const view = await api(`/api/cases/${encodeURIComponent(id)}`); state.view = view; state.case = view.case; render(); localStorage.setItem('closure-case', id); await loadOverview(); }

function render() {
  const current = state.case; $('#overview').hidden = true; $('#empty').hidden = true; $('#workspace').hidden = false;
  $('#case-title').textContent = current.case_id; $('#case-meta').textContent = `${current.site_code} · ${current.trench_coordinates} · 修订 ${current.revision}`; $('#status').textContent = statusText[current.status] || current.status;
  const panel = $('#integrity-panel'); panel.hidden = current.status === 'draft';
  if (!panel.hidden) { const integrity = state.view.baseline_integrity; $('#integrity').innerHTML = integrity.valid ? `<span class="ok">正常 · 凭证 ${esc(current.baseline?.receipt_digest)}</span>` : `<span class="bad">异常，方案操作已隐藏</span>${integrity.differences.map((item) => `<p>${esc(item.field)}：${esc(item.message)}</p>`).join('')}`; }
  renderProfile(); renderActions(); renderDefects(); renderTimeline();
}
function renderProfile() {
  const current = state.case; const profile = $('#profile'); profile.innerHTML = ''; const layers = current.plan?.layers || []; $('#progress').textContent = `${state.view.summary.passed_layers} / ${layers.length} 层合格`;
  if (!layers.length) { profile.innerHTML = '<div class="layer pending">方案尚未批准</div>'; return; }
  layers.forEach((layer) => {
    const execution = current.executions?.find((item) => item.layer_index === layer.index); const element = document.createElement('div'); element.className = `layer ${execution?.verdict || 'pending'}`; const actual = execution ? `${execution.actual_thickness_mm} mm / ${execution.compaction_percent}%` : `目标 ${layer.target_thickness_mm} mm`;
    const rules = execution?.evaluation?.rules?.map((rule) => `<tr class="${rule.passed ? 'ok' : 'bad'}"><td>${esc(rule.code)}</td><td>${esc(rule.actual)}</td><td>${esc(rule.requirement)}</td><td>${rule.margin == null ? '—' : rule.margin}</td></tr>`).join('') || '';
    element.innerHTML = `<div><strong>第 ${layer.index} 层 · ${esc(layer.material_code)}</strong><span>${actual}</span></div>${rules ? `<details><summary>判定明细 · ${esc(execution.evaluation.rule_version)}</summary><table><thead><tr><th>规则</th><th>输入</th><th>要求</th><th>余量</th></tr></thead><tbody>${rules}</tbody></table></details>` : ''}`; profile.appendChild(element);
  });
}

function renderActions() {
  const current = state.case; const actions = $('#actions'); actions.innerHTML = '';
  if (current.status === 'draft') { actions.append(template('#freeze-form')); const form = $('form', actions); $('[data-precheck]', form).onclick = () => precheckBaseline(form); form.onsubmit = submitAction; }
  else if (!state.view.baseline_integrity.valid) actions.innerHTML = '<p class="bad">冻结基线完整性异常，禁止继续编制或批准方案。</p>';
  else if (current.status === 'baseline_frozen' && !current.plan) { actions.append(template('#plan-form')); buildLayers($('[name=layer_count]', actions), $('#layer-fields', actions)); $('form', actions).onsubmit = submitAction; }
  else if (current.status === 'baseline_frozen') { actions.append(template('#approve-form')); renderPlanReview(actions); $('form', actions).onsubmit = submitAction; }
  else if (['plan_approved', 'in_progress'].includes(current.status)) { actions.append(template('#execute-form')); setupDraftForm($('form', actions)); }
  else if (current.status === 'awaiting_review') { actions.append(template('#review-form')); setupReviewForm($('form', actions)); }
  else if (current.status === 'remediation') actions.innerHTML = '<p>请在缺陷待办中按当前阶段处理开放缺陷。</p>';
  else if (current.status === 'sealed') {
    actions.innerHTML = `<div class="sealed"><a href="/api/cases/${encodeURIComponent(current.case_id)}/dossier">下载封护档案</a><button id="verify">生成分项校验报告</button><a href="/api/cases/${encodeURIComponent(current.case_id)}/verification-report?download=1">下载校验报告</a><div id="report"></div></div>`;
    $('#verify').onclick = async () => { try { const report = await api(`/api/cases/${encodeURIComponent(current.case_id)}/verification-report`); $('#report').innerHTML = `<p class="${report.valid ? 'ok' : 'bad'}">${report.valid ? '全部检查通过' : '存在校验失败'}</p>${report.checks.map((check) => `<p class="${check.passed ? 'ok' : 'bad'}">${esc(check.code)} · ${esc(check.message)}</p>`).join('')}`; } catch (error) { notice(error.message, true); } };
  }
}
async function precheckBaseline(form) {
  const data = formData(form); const people = data.people.split(',').map((item) => item.trim());
  try { const result = await api(`/api/cases/${encodeURIComponent(state.case.case_id)}/baseline/precheck`, { method: 'POST', body: JSON.stringify({ actor: data.actor, people }) }); $('[name=confirmed_digest]', form).value = result.can_freeze ? result.summary_digest : ''; $('#precheck', form).innerHTML = result.can_freeze ? `<span class="ok">预检通过，待确认摘要：${esc(result.summary_digest)}</span>` : result.issues.map((item) => `<p class="bad">${esc(item.field)}：${esc(item.message)}</p>`).join(''); } catch (error) { notice(error.message, true); }
}
function renderPlanReview(root) {
  const review = state.case.plan.review; $('#plan-review', root).innerHTML = `<strong>总目标厚度 ${review.total_thickness_mm} mm</strong>${review.layers.map((item) => `<p>第 ${item.layer_index} 层：${esc(item.summary)}${item.changes?.length ? `；${esc(item.changes.join('；'))}` : ''}</p>`).join('')}`;
  $('#risk-list', root).innerHTML = review.risks.length ? review.risks.map((risk) => `<label class="risk"><input type="checkbox" name="risk" value="${esc(risk.code)}">第 ${risk.layer_index} 层 · ${esc(risk.message)} (${esc(risk.code)})</label>`).join('') : '<span class="ok">未发现风险提示</span>';
}
function buildLayers(input, box) { const draw = () => { box.innerHTML = ''; for (let index = 1; index <= Number(input.value); index += 1) { const row = document.createElement('div'); row.className = 'layer-spec'; row.innerHTML = `<input name="material_${index}" value="clean-soil-${index}" placeholder="材料"><input name="thickness_${index}" type="number" value="200" placeholder="厚度"><input name="tolerance_${index}" type="number" value="10" placeholder="容差"><input name="moisture_${index}" value="12,18" placeholder="含水率范围"><input name="compaction_${index}" type="number" value="90" placeholder="压实阈值">`; box.append(row); } }; input.onchange = draw; draw(); }
function planLayers(data) { return Array.from({ length: Number(data.layer_count) }, (_, offset) => { const index = offset + 1; const moisture = data[`moisture_${index}`].split(',').map(Number); return { index, material_code: data[`material_${index}`], target_thickness_mm: Number(data[`thickness_${index}`]), thickness_tolerance_mm: Number(data[`tolerance_${index}`]), moisture_min_percent: moisture[0], moisture_max_percent: moisture[1], compaction_min_percent: Number(data[`compaction_${index}`]), evidence_required: true }; }); }

function setupDraftForm(form) {
  const current = state.case; const index = (current.executions?.length || 0) + 1; const spec = current.plan.layers[index - 1]; const draft = state.view.layer_draft; $('#expected-layer', form).textContent = `第 ${index} 层：${spec.material_code}，${spec.target_thickness_mm}±${spec.thickness_tolerance_mm} mm，含水率 ${spec.moisture_min_percent}-${spec.moisture_max_percent}%，压实度 ≥${spec.compaction_min_percent}%`;
  const values = draft || { material_code: spec.material_code }; ['material_code', 'actual_thickness_mm', 'moisture_percent', 'compaction_percent', 'performed_by', 'evidence_digest'].forEach((name) => { if (values[name] != null) $(`[name=${name}]`, form).value = values[name]; }); $('#draft-check', form).textContent = draft ? `草稿 v${draft.draft_version} · 待补：${draft.missing_fields.join('、') || '无'}` : '尚未保存草稿'; form.onsubmit = submitDraft;
}
function optionalNumber(value) { return value === '' ? undefined : Number(value); }
async function submitDraft(event) {
  event.preventDefault(); const form = event.currentTarget; const data = formData(form); const current = state.case; const draft = state.view.layer_draft; const body = { layer_index: (current.executions?.length || 0) + 1, material_code: data.material_code, actual_thickness_mm: optionalNumber(data.actual_thickness_mm), moisture_percent: optionalNumber(data.moisture_percent), compaction_percent: optionalNumber(data.compaction_percent), performed_by: data.performed_by, evidence_digest: data.evidence_digest, expected_draft_version: draft?.draft_version || 0 };
  try { await api(`/api/cases/${encodeURIComponent(current.case_id)}/layer-draft`, { method: 'PUT', body: JSON.stringify(body) }); await loadCase(current.case_id); if (event.submitter.value === 'save') { notice('草稿已保存，案件修订未变化'); return; } const check = await api(`/api/cases/${encodeURIComponent(current.case_id)}/layer-draft/check`); if (!check.can_submit) { notice(`核对未通过：${check.issues.map((item) => item.field).join('、')}`, true); return; } if (!window.confirm('方案目标与草稿实绩核对通过，确认正式提交？')) return; await api(`/api/cases/${encodeURIComponent(current.case_id)}/layer-draft/submit`, { method: 'POST', body: JSON.stringify({ request_id: uid(), execution_id: uid(), expected_revision: state.case.revision, draft_version: state.view.layer_draft.draft_version }) }); await loadCase(current.case_id); notice('施工层已正式提交'); } catch (error) { notice(error.message, true); }
}

function setupReviewForm(form) {
  const select = $('[name=decision]', form); const box = $('#review-issues', form); const add = $('#add-issue', form); const appendIssue = () => { const row = document.createElement('div'); row.className = 'issue-row'; row.innerHTML = `<input name="issue_layer" type="number" min="1" max="${state.case.plan.layers.length}" placeholder="关联层" required><input name="issue_description" placeholder="问题说明" required><input name="issue_action" placeholder="要求的纠正措施" required><input name="issue_evidence" placeholder="复核证据类型" required><button type="button" title="删除问题">×</button>`; $('button', row).onclick = () => row.remove(); box.append(row); }; const toggle = () => { const returned = select.value === 'return'; box.hidden = !returned; add.hidden = !returned; box.querySelectorAll('input').forEach((input) => { input.required = returned; }); if (returned && !box.children.length) appendIssue(); }; select.onchange = toggle; add.onclick = appendIssue; toggle(); form.onsubmit = submitAction;
}
async function submitAction(event) {
  event.preventDefault(); const form = event.currentTarget; const data = formData(form); const current = state.case; const base = { request_id: uid(), expected_revision: current.revision }; let path; let body;
  if (form.dataset.action === 'baseline') { if (!data.confirmed_digest) { notice('请先完成无阻断项的冻结预检', true); return; } path = 'baseline'; body = { ...base, actor: data.actor, people: data.people.split(',').map((item) => item.trim()), confirmed_digest: data.confirmed_digest }; }
  else if (form.dataset.action === 'plan') { path = 'plan'; body = { ...base, prepared_by: data.prepared_by, layers: planLayers(data) }; }
  else if (form.dataset.action === 'approve') { path = 'approve'; body = { ...base, actor: data.actor, plan_digest: current.plan.plan_digest, confirmed_risk_codes: Array.from(form.querySelectorAll('[name=risk]:checked')).map((item) => item.value) }; }
  else { const rows = Array.from(form.querySelectorAll('.issue-row')); path = 'review'; body = { ...base, actor: data.actor, decision: data.decision, issues: data.decision === 'return' ? rows.map((row) => ({ layer_index: Number($('[name=issue_layer]', row).value), description: $('[name=issue_description]', row).value, required_action: $('[name=issue_action]', row).value, required_evidence_type: $('[name=issue_evidence]', row).value })) : [] }; }
  try { await api(`/api/cases/${encodeURIComponent(current.case_id)}/${path}`, { method: 'POST', body: JSON.stringify(body) }); await loadCase(current.case_id); notice('操作已提交'); } catch (error) { notice(error.message, true); }
}

function renderDefects() {
  const current = state.case; const box = $('#defects'); const open = (current.defects || []).filter((item) => item.status === 'open'); $('#defect-count').textContent = `${open.length} 项开放`; box.innerHTML = ''; if (!open.length) { box.textContent = '当前没有开放缺陷'; return; }
  open.forEach((defect) => {
    const element = document.createElement('div'); element.className = 'defect'; const attempts = defect.retest_attempts?.map((item, index) => `<p class="${item.passed ? 'ok' : 'bad'}">复验 ${index + 1}：${item.passed ? '通过' : `未通过 ${esc(item.remaining_rules.join('、'))}`} · ${esc(item.evidence_digest)}</p>`).join('') || ''; let formHTML;
    if (!defect.plans?.length) formHTML = '<form data-stage="plan"><input name="actor" placeholder="登记人" required><input name="category" placeholder="原因分类" required><input name="cause" placeholder="原因说明" required><input name="action" placeholder="纠正措施" required><input name="responsible" placeholder="责任人" required><input name="planned" type="datetime-local" required><button class="primary">登记整改方案</button></form>';
    else if (!defect.completion) formHTML = '<form data-stage="complete"><input name="actor" placeholder="完成人" required><input name="description" placeholder="实际完成说明" required><input name="evidence" placeholder="完成证据摘要" required><button class="primary">记录措施完成</button></form>';
    else { const inputs = defect.failed_rule_codes.map((code) => retestInput(code)).join(''); formHTML = `<form data-stage="retest"><input name="actor" placeholder="复验人" required><input name="evidence" placeholder="复验证据摘要" required>${inputs}<button class="primary">提交定向复验</button></form>`; }
    element.innerHTML = `<strong>第 ${defect.layer_index} 层 · ${defect.source === 'review' ? '验收退回' : '施工判定'}</strong><p>${esc(defect.failed_rule_codes.join('、'))}</p>${attempts}${formHTML}`; $('form', element).onsubmit = (event) => submitDefect(event, defect); box.append(element);
  });
}
function retestInput(code) { const fields = { MATERIAL_MISMATCH: ['material_confirmed', '材料复核完成'], THICKNESS_OUT_OF_RANGE: ['thickness_mm', '复验厚度 mm'], MOISTURE_OUT_OF_RANGE: ['moisture_percent', '复验含水率 %'], COMPACTION_BELOW_THRESHOLD: ['compaction_percent', '复验压实度 %'], EVIDENCE_REQUIRED: ['evidence_confirmed', '证据补齐确认'], REVIEW_RETURNED: ['review_confirmed', '验收问题完成确认'] }; const [name, label] = fields[code] || [code.toLowerCase(), code]; return `<label>${esc(label)}<input name="${esc(name)}" type="number" step="0.1"${name.endsWith('confirmed') ? ' min="1" max="1" value="1"' : ''} required></label>`; }
async function submitDefect(event, defect) {
  event.preventDefault(); const form = event.currentTarget; const data = formData(form); const caseID = state.case.case_id; const base = `/api/cases/${encodeURIComponent(caseID)}/defects/${encodeURIComponent(defect.defect_id)}`; let path; let body = { request_id: uid(), expected_revision: state.case.revision, actor: data.actor };
  if (form.dataset.stage === 'plan') { path = 'plan'; body = { ...body, cause_category: data.category, cause: data.cause, corrective_action: data.action, responsible: data.responsible, planned_completion_at: new Date(data.planned).toISOString() }; }
  else if (form.dataset.stage === 'complete') { path = 'complete'; body = { ...body, description: data.description, evidence_digest: data.evidence }; }
  else { path = 'retest'; const values = {}; Object.entries(data).forEach(([key, value]) => { if (key !== 'actor' && key !== 'evidence') values[key] = Number(value); }); body = { ...body, evidence_digest: data.evidence, retest_values: values }; }
  try { await api(`${base}/${path}`, { method: 'POST', body: JSON.stringify(body) }); await loadCase(caseID); const latest = state.case.defects.find((item) => item.defect_id === defect.defect_id); notice(latest?.status === 'closed' ? '缺陷已通过复验并关闭' : '阶段记录已保存'); } catch (error) { notice(error.message, true); }
}
function renderTimeline() { const list = $('#timeline'); list.innerHTML = ''; [...state.view.timeline].reverse().forEach((event) => { const item = document.createElement('li'); item.innerHTML = `<strong>${esc(event.type)}</strong><span> · ${esc(event.actor)}</span><time>${new Date(event.at).toLocaleString()} · r${event.revision}</time>`; list.append(item); }); }

$('#create').onsubmit = async (event) => { event.preventDefault(); const data = formData(event.currentTarget); data.request_id = uid(); try { await api('/api/cases', { method: 'POST', body: JSON.stringify(data) }); await loadCase(data.case_id); notice('案件已建立'); } catch (error) { notice(error.message, true); } };
$('#filters').onsubmit = (event) => { event.preventDefault(); loadOverview().catch((error) => notice(error.message, true)); };
$('#new-case').onclick = () => { $('#overview').hidden = true; $('#workspace').hidden = true; $('#empty').hidden = false; };
$('#cancel-create').onclick = () => { $('#empty').hidden = true; $('#overview').hidden = false; };
$('#back-overview').onclick = () => { $('#workspace').hidden = true; $('#overview').hidden = false; loadOverview().catch((error) => notice(error.message, true)); };
$('#lookup').onsubmit = async (event) => { event.preventDefault(); try { await loadCase($('#case-id').value.trim()); } catch (error) { notice(error.message, true); } };
$('#refresh').onclick = () => state.case && loadCase(state.case.case_id).catch((error) => notice(error.message, true));
loadOverview().catch((error) => notice(error.message, true));
