const ACCOUNT_ID = "acc-fatec-01";
const API_KEY = "console-web";
let currentMode = "REST"; // "REST" ou "GRAPHQL"
let cachedSGs = [];
let hideTerminated = false;

function toggleHideTerminated(chk) {
  hideTerminated = chk.checked;
  refreshData();
}

// Inicialização
document.addEventListener("DOMContentLoaded", () => {
  refreshData();
  // Polling automático a cada 5 segundos para faturamento em tempo real
  setInterval(refreshData, 5000);
});

function switchMode(mode) {
  currentMode = mode;
  const btnRest = document.getElementById("btn-mode-rest");
  const btnGql = document.getElementById("btn-mode-graphql");
  const desc = document.getElementById("mode-desc");

  if (mode === "REST") {
    btnRest.className = "px-3 py-1 text-xs font-semibold rounded-md bg-amber-500 text-slate-950 transition";
    btnGql.className = "px-3 py-1 text-xs font-semibold rounded-md text-slate-300 hover:text-white transition";
    desc.innerText = "Rota agregada GET /api/v1/dashboard/acc-fatec-01 com degradação graciosa";
  } else {
    btnGql.className = "px-3 py-1 text-xs font-semibold rounded-md bg-amber-500 text-slate-950 transition";
    btnRest.className = "px-3 py-1 text-xs font-semibold rounded-md text-slate-300 hover:text-white transition";
    desc.innerText = "BFF GraphQL POST /graphql com consulta única accountDashboard(accountId: 'acc-fatec-01')";
  }
  refreshData();
}

async function refreshData() {
  if (currentMode === "REST") {
    await fetchRestDashboard();
  } else {
    await fetchGraphQLDashboard();
  }
}

// 1. Consulta via Rota Agregada REST do Gateway
async function fetchRestDashboard() {
  try {
    const res = await fetch(`/api/v1/dashboard/${ACCOUNT_ID}`, {
      headers: {
        "X-API-Key": API_KEY,
        "Accept": "application/json"
      }
    });

    const reqId = res.headers.get("X-Request-Id");
    if (reqId) document.getElementById("current-request-id").innerText = reqId;

    if (!res.ok) {
      console.error("Falha ao consultar dashboard REST:", res.status);
      return;
    }

    const data = await res.json();
    renderDashboard({
      instances: data.compute ? data.compute.items || [] : [],
      securityGroups: data.security ? data.security.items || [] : [],
      budget: data.billing || null,
      degraded: data.degraded_services || []
    });
  } catch (err) {
    console.error("Erro na requisição REST:", err);
  }
}

// 2. Consulta via GraphQL BFF
async function fetchGraphQLDashboard() {
  const query = `
    query GetDashboard($accountId: String!) {
      accountDashboard(accountId: $accountId) {
        accountId
        totalInstances
        runningInstances
        totalVCPUs
        instances {
          id
          name
          flavor
          status
          ipPrivado
          ipPublico
          vpcId
          securityGroupId
          vcpus
          memoryGb
          createdAt
        }
        securityGroups {
          id
          name
          vpcId
          rules {
            id
            protocol
            startPort
            endPort
            cidrOrigin
          }
        }
        budget {
          currentCost
          budgetLimit
          currency
          exceeded
          forecastedCost
        }
      }
    }
  `;

  try {
    const res = await fetch(`/graphql`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-API-Key": API_KEY,
        "Accept": "application/json"
      },
      body: JSON.stringify({
        query: query,
        variables: { accountId: ACCOUNT_ID }
      })
    });

    const reqId = res.headers.get("X-Request-Id");
    if (reqId) document.getElementById("current-request-id").innerText = reqId;

    const result = await res.json();
    if (result.data && result.data.accountDashboard) {
      const d = result.data.accountDashboard;
      renderDashboard({
        instances: d.instances || [],
        securityGroups: d.securityGroups || [],
        budget: d.budget || null,
        degraded: []
      });
    }
  } catch (err) {
    console.error("Erro na requisição GraphQL:", err);
  }
}

// 3. Renderização no DOM
function renderDashboard({ instances, securityGroups, budget, degraded }) {
  cachedSGs = securityGroups;

  // Atualizar KPIs
  const total = instances.length;
  const running = instances.filter(i => i.status === "RUNNING").length;
  let totalVCPUs = 0;
  instances.forEach(i => {
    if (i.status === "RUNNING" || i.status === "PROVISIONING") {
      totalVCPUs += (i.vcpus || (i.flavor === "c5.large" ? 2 : 1));
    }
  });

  document.getElementById("kpi-instances-count").innerText = total;
  document.getElementById("kpi-running-badge").innerText = `${running} Ativas`;
  document.getElementById("kpi-vcpus-count").innerHTML = `${totalVCPUs} <span class="text-sm font-normal text-slate-400">/ 8</span>`;

  const vcpuPct = Math.min(100, Math.round((totalVCPUs / 8) * 100));
  document.getElementById("kpi-vcpus-pct").innerText = `${vcpuPct}%`;
  const vcpuBar = document.getElementById("kpi-vcpu-bar");
  vcpuBar.style.width = `${vcpuPct}%`;
  vcpuBar.className = vcpuPct >= 100 ? "bg-red-500 h-1.5 rounded-full" : "bg-amber-500 h-1.5 rounded-full";

  document.getElementById("kpi-sgs-count").innerText = securityGroups.length;

  // Atualizar Faturamento e Alerta de Custo
  if (budget) {
    const cost = budget.current_cost !== undefined ? budget.current_cost : budget.currentCost;
    const limit = budget.budget_limit !== undefined ? budget.budget_limit : budget.budgetLimit;
    const exceeded = budget.exceeded;

    document.getElementById("kpi-cost").innerText = `$ ${cost.toFixed(2)}`;
    document.getElementById("kpi-limit").innerText = `$ ${limit.toFixed(2)}`;

    const pct = Math.min(100, Math.round((cost / limit) * 100));
    const pBar = document.getElementById("budget-progress-bar");
    pBar.style.width = `${pct}%`;

    const banner = document.getElementById("budget-alert-banner");
    if (exceeded) {
      banner.classList.remove("hidden");
      pBar.className = "bg-red-500 h-2 rounded-full";
      document.getElementById("card-billing").classList.add("border-red-500", "shadow-red-900/30");
    } else {
      banner.classList.add("hidden");
      pBar.className = pct > 80 ? "bg-amber-500 h-2 rounded-full" : "bg-emerald-500 h-2 rounded-full";
      document.getElementById("card-billing").classList.remove("border-red-500", "shadow-red-900/30");
    }
  }

  // Tabela de Instâncias
  const tbody = document.getElementById("instances-tbody");
  const displayInstances = hideTerminated ? instances.filter(i => i.status !== "TERMINATED") : instances;

  if (displayInstances.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" class="text-center py-8 text-slate-500">Nenhuma instância para exibir. Clique em "Lançar Instância" para começar.</td></tr>`;
  } else {
    tbody.innerHTML = displayInstances.map(inst => {
      const isRunning = inst.status === "RUNNING";
      const isStopped = inst.status === "STOPPED";
      const isTerminated = inst.status === "TERMINATED";

      let statusBadge = "";
      if (isRunning) statusBadge = `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-950/80 text-emerald-400 border border-emerald-800/60">RUNNING</span>`;
      else if (isStopped) statusBadge = `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-amber-950/80 text-amber-400 border border-amber-800/60">STOPPED</span>`;
      else if (isTerminated) statusBadge = `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-slate-800 text-slate-400 border border-slate-700">TERMINATED</span>`;
      else statusBadge = `<span class="px-2 py-0.5 rounded text-[10px] font-bold bg-blue-950 text-blue-400">${inst.status}</span>`;

      return `
        <tr class="hover:bg-slate-800/40 transition">
          <td class="px-5 py-3">
            <div class="font-bold text-white text-xs">${inst.name}</div>
            <div class="text-[10px] font-mono text-slate-400">${inst.id}</div>
          </td>
          <td class="px-5 py-3">${statusBadge}</td>
          <td class="px-5 py-3 font-mono text-amber-300">${inst.flavor}</td>
          <td class="px-5 py-3 text-slate-300">${inst.vcpus || (inst.flavor === 'c5.large' ? 2 : 1)} vCPU / ${inst.memoryGb || (inst.flavor === 'c5.large' ? 4 : 1)} GB</td>
          <td class="px-5 py-3 font-mono text-slate-300">${inst.ip_privado || inst.ipPrivado || '-'}</td>
          <td class="px-5 py-3 font-mono text-slate-200">${inst.ip_publico || inst.ipPublico || '<span class="text-slate-600">-</span>'}</td>
          <td class="px-5 py-3 font-mono text-xs text-blue-400">${inst.security_group_id || inst.securityGroupId}</td>
          <td class="px-5 py-3 text-right space-x-1.5 whitespace-nowrap">
            ${isRunning ? `
              <button onclick="changeInstanceStatus('${inst.id}', 'STOPPED')" title="Derrubar / Forçar Parada da VM" class="px-2.5 py-1 rounded bg-amber-950/80 hover:bg-amber-800 text-amber-300 border border-amber-700/60 text-[11px] font-bold transition">⚡ Derrubar</button>
              <button onclick="terminateInstance('${inst.id}')" title="Terminar e Desalocar Recursos" class="px-2 py-1 rounded bg-red-950/80 hover:bg-red-800 text-red-300 border border-red-700/60 text-[11px] font-semibold transition">🗑️ Terminar</button>
            ` : ''}
            ${isStopped ? `
              <button onclick="changeInstanceStatus('${inst.id}', 'RUNNING')" title="Iniciar VM" class="px-2.5 py-1 rounded bg-emerald-950/80 hover:bg-emerald-800 text-emerald-300 border border-emerald-700/60 text-[11px] font-bold transition">▶️ Iniciar</button>
              <button onclick="terminateInstance('${inst.id}')" title="Terminar e Desalocar Recursos" class="px-2 py-1 rounded bg-red-950/80 hover:bg-red-800 text-red-300 border border-red-700/60 text-[11px] font-semibold transition">🗑️ Terminar</button>
            ` : ''}
            ${isTerminated ? `
              <button onclick="purgeInstance('${inst.id}')" title="Remover definitivamente da lista" class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-600 text-[11px] font-semibold transition">❌ Limpar</button>
            ` : ''}
          </td>
        </tr>
      `;
    }).join("");
  }

  // Renderizar Security Groups
  const sgContainer = document.getElementById("sg-container");
  if (securityGroups.length === 0) {
    sgContainer.innerHTML = `<div class="text-slate-500 text-xs col-span-2">Nenhum Security Group cadastrado.</div>`;
  } else {
    sgContainer.innerHTML = securityGroups.map(sg => {
      const rules = sg.rules || [];
      return `
        <div class="bg-slate-950/60 border border-slate-800 rounded-lg p-3 space-y-2">
          <div class="flex justify-between items-center">
            <span class="font-bold text-white text-xs">${sg.name}</span>
            <span class="text-[10px] font-mono text-slate-400">${sg.id}</span>
          </div>
          <div class="text-[11px] text-slate-400">VPC: <code class="text-slate-300 font-mono">${sg.vpc_id || sg.vpcId}</code></div>
          <div class="pt-1 border-t border-slate-800/80 space-y-1">
            <div class="text-[10px] font-semibold uppercase text-slate-500">Regras de Ingresso:</div>
            ${rules.map(r => `
              <div class="flex items-center justify-between text-[11px] bg-slate-900 px-2 py-1 rounded border border-slate-800">
                <span class="font-mono text-amber-400 font-bold">${r.protocol}</span>
                <span class="text-slate-300">Portas: <strong>${r.start_port || r.startPort}</strong> &rarr; <strong>${r.end_port || r.endPort}</strong></span>
                <span class="text-slate-400 font-mono text-[10px]">${r.cidr_origin || r.cidrOrigin}</span>
              </div>
            `).join("")}
          </div>
        </div>
      `;
    }).join("");
  }
}

// 4. Ações de Instância
async function changeInstanceStatus(id, newStatus) {
  try {
    const res = await fetch(`/v1/instances/${id}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        "X-API-Key": API_KEY
      },
      body: JSON.stringify({ status: newStatus })
    });
    if (!res.ok) {
      const prob = await res.json();
      alert(`[Erro ${prob.status}] ${prob.title}: ${prob.detail}`);
    }
    refreshData();
  } catch (err) {
    alert("Falha na comunicação com o Gateway: " + err);
  }
}

async function terminateInstance(id) {
  if (!confirm("Tem certeza que deseja terminar e excluir esta instância virtual?")) return;
  try {
    const res = await fetch(`/v1/instances/${id}`, {
      method: "DELETE",
      headers: { "X-API-Key": API_KEY }
    });
    if (!res.ok) {
      const prob = await res.json();
      alert(`[Erro ${prob.status}] ${prob.title}: ${prob.detail}`);
    }
    refreshData();
  } catch (err) {
    alert("Falha ao excluir: " + err);
  }
}

// 5. Modal de Lançamento de Instância
function openLaunchModal() {
  const sgSelect = document.getElementById("inst-sg");
  sgSelect.innerHTML = cachedSGs.map(sg => `<option value="${sg.id}">${sg.name} (${sg.id})</option>`).join("");
  document.getElementById("launch-error").classList.add("hidden");
  document.getElementById("launch-modal").classList.remove("hidden");
}

function closeLaunchModal() {
  document.getElementById("launch-modal").classList.add("hidden");
}

async function handleLaunchInstance(e) {
  e.preventDefault();
  const name = document.getElementById("inst-name").value;
  const flavor = document.getElementById("inst-flavor").value;
  const vpc_id = document.getElementById("inst-vpc").value;
  const security_group_id = document.getElementById("inst-sg").value;
  const errDiv = document.getElementById("launch-error");

  try {
    const res = await fetch("/v1/instances", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-API-Key": API_KEY
      },
      body: JSON.stringify({
        name,
        flavor,
        vpc_id,
        security_group_id,
        account_id: ACCOUNT_ID
      })
    });

    if (!res.ok) {
      const prob = await res.json();
      errDiv.innerText = `[RFC 9457 - ${prob.status}] ${prob.title}: ${prob.detail}`;
      errDiv.classList.remove("hidden");
      return;
    }

    closeLaunchModal();
    document.getElementById("launch-form").reset();
    refreshData();
  } catch (err) {
    errDiv.innerText = "Erro ao enviar requisição: " + err;
    errDiv.classList.remove("hidden");
  }
}

// 6. Injeção de Custo (Cena 7)
function openSimulateModal() {
  document.getElementById("simulate-modal").classList.remove("hidden");
}

function closeSimulateModal() {
  document.getElementById("simulate-modal").classList.add("hidden");
}

async function handleSimulateCharge() {
  const amount = parseFloat(document.getElementById("sim-amount").value);
  try {
    const res = await fetch(`/v1/budgets/${ACCOUNT_ID}/simulate-charge`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-API-Key": API_KEY
      },
      body: JSON.stringify({ amount, reason: "Teste automatizado da Cena 7" })
    });
    if (res.ok) {
      closeSimulateModal();
      refreshData();
    } else {
      alert("Falha ao simular consumo.");
    }
  } catch (err) {
    alert("Erro: " + err);
  }
}

function dismissAlert() {
  document.getElementById("budget-alert-banner").classList.add("hidden");
}

// 7. Purgar Instância Terminada
async function purgeInstance(id) {
  try {
    const res = await fetch(`/v1/instances/${id}?purge=true`, {
      method: "DELETE",
      headers: { "X-API-Key": API_KEY }
    });
    if (res.ok || res.status === 204 || res.status === 404) {
      refreshData();
    } else {
      const prob = await res.json();
      alert(`[Erro ${prob.status}] ${prob.title}: ${prob.detail}`);
    }
  } catch (err) {
    alert("Erro ao purgar instância: " + err);
  }
}

// 8. Limpar Todas as Instâncias Terminadas
async function handlePurgeTerminated() {
  if (!confirm("Deseja remover todas as instâncias em estado TERMINATED do histórico do console?")) return;
  try {
    const res = await fetch(`/v1/instances/purge?account_id=${ACCOUNT_ID}`, {
      method: "POST",
      headers: { "X-API-Key": API_KEY }
    });
    if (res.ok) {
      refreshData();
    } else {
      alert("Falha ao limpar instâncias terminadas.");
    }
  } catch (err) {
    alert("Erro na requisição: " + err);
  }
}

// 9. Resetar Fatura / Zerar Custo Simulado
async function handleResetBudget() {
  if (!confirm("Deseja zerar os consumos simulados da fatura e recalcular o saldo real?")) return;
  try {
    const res = await fetch(`/v1/budgets/${ACCOUNT_ID}/reset`, {
      method: "POST",
      headers: { "X-API-Key": API_KEY }
    });
    if (res.ok) {
      dismissAlert();
      refreshData();
    } else {
      alert("Falha ao resetar fatura.");
    }
  } catch (err) {
    alert("Erro ao resetar: " + err);
  }
}

