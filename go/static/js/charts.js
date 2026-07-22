// Pilot Finance — Chart.js initialization (projection + pie)
// Loaded only on dashboard page
//
// NOTE on Chart.js bundle size (chart.umd.min.js ≈ 203 KB):
// This project uses the full UMD build loaded via <script> tag (no bundler).
// Tree-shaking requires ESM imports + a bundler (webpack/vite/rollup).
// Chart types used: line (projection) and doughnut (pie/allocation).
// Plugins used: Filler (area fills), Tooltip, Legend (hidden).
// Switching to a bundler solely for Chart.js would be overengineering for
// this project. If a bundler is introduced later, replace with:
//   import { Chart, LineController, DoughnutController, LineElement,
//            ArcElement, PointElement, CategoryScale, LinearScale,
//            Filler, Tooltip, Legend } from 'chart.js';
//   Chart.register(LineController, DoughnutController, LineElement,
//            ArcElement, PointElement, CategoryScale, LinearScale,
//            Filler, Tooltip, Legend);
// This would reduce the Chart.js payload to ~80 KB.

// Money formatting comes from the shared window.PILOT_FMT (defined in base.html)
// so axis labels and the pie-center compact value stay in sync with dashboard.html.
const fmt = v => window.PILOT_FMT.currency(v);
const fmtAxis = v => window.PILOT_FMT.compact(v);
const getColors = () => {
    const d = document.documentElement.classList.contains('dark');
    return { isDark: d, grid: d ? 'rgba(148,163,184,.1)' : 'rgba(100,116,139,.1)', text: d ? '#94a3b8' : '#64748b', tipBg: d ? '#1e293b' : '#fff', tipTitle: d ? '#f1f5f9' : '#0f172a', tipBody: d ? '#cbd5e1' : '#475569', tipBorder: d ? '#334155' : '#e2e8f0' };
};
const dsOpts = (c) => ({ backgroundColor: c, borderWidth: 0, fill: true, tension: .3, pointRadius: 0, pointHoverRadius: 4, pointBackgroundColor: c, pointBorderWidth: 0 });

// Detect cone data (variable-rate accounts)
const hasCone = (data) => data?.some(d => d.totalMin !== d.totalAvg);

// Create projection datasets
const createDS = (data, acc) => {
    const cone = hasCone(data);
    const baseDS = acc?.length && data[0]?.accounts
        ? acc.map(a => ({ label: a.name, data: data.map(d => d.accounts?.[a.name] || 0), ...dsOpts(a.color), stack: 'accounts' }))
        : [{ label: 'Projection', data: data.map(d => d.totalAvg), ...dsOpts('#3b82f6'), stack: 'accounts' }];
    if (!cone) return baseDS;
    const co = { pointRadius: 0, pointHoverRadius: 0, tension: 0.4 };
    const accDS = baseDS.map(d => ({ ...d, order: 2 }));
    const isDark = document.documentElement.classList.contains('dark');
    const minColor = isDark ? 'rgba(255,255,255,0.75)' : 'rgba(15,23,42,0.6)';
    return [
        { label: '_maxFill', data: data.map(d => d.totalMax), fill: '+1', backgroundColor: 'rgba(16,185,129,0.18)', borderWidth: 0, stack: 'gmax', order: 3, ...co },
        { label: '_avgRef',  data: data.map(d => d.totalAvg), fill: false,  borderWidth: 0, stack: 'gavg', order: 3, ...co },
        ...accDS,
        { label: '_maxLine', data: data.map(d => d.totalMax), fill: false, borderColor: 'rgba(16,185,129,0.85)', borderWidth: 1.5, borderDash: [5, 3], stack: 'lmax', order: 1, ...co },
        { label: '_minLine', data: data.map(d => d.totalMin), fill: false, borderColor: minColor, borderWidth: 2, borderDash: [4, 4], stack: 'lmin', order: 0, ...co },
    ];
};

// Auto-dismiss tooltip after 3s (mobile)
const tooltipTimeoutPlugin = {
    id: 'tooltipTimeout',
    afterEvent(chart, args) {
        if (args.event.type === 'click') {
            clearTimeout(chart._ttTimer);
            chart._ttTimer = setTimeout(() => {
                chart.tooltip.setActiveElements([], {x:0,y:0});
                chart.update('none');
            }, 3000);
        }
    }
};

// Projection chart
window.initProjectionChart = (data, acc) => {
    window._projData = data; window._projAcc = acc;
    const ctx = document.getElementById('projectionCanvas');
    if (!ctx || typeof Chart === 'undefined') return;
    if (window.projectionChart) { clearTimeout(window.projectionChart._ttTimer); window.projectionChart.destroy(); }
    if (!data?.length) {
        const noDataText = ctx.dataset.nodata || 'No data';
        ctx.parentElement.innerHTML = '<div class="h-full flex items-center justify-center text-muted-foreground">'+noDataText+'</div>';
        return;
    }
    const c = getColors();
    const cone = hasCone(data);
    window.projectionChart = new Chart(ctx, {
        type: 'line',
        plugins: [tooltipTimeoutPlugin],
        data: { labels: data.map(d => d.name || 'An '+d.year), datasets: createDS(data, acc) },
        options: {
            responsive: true, maintainAspectRatio: false, animation: { duration: 400, easing: 'easeOutQuart' }, interaction: { intersect: false, mode: 'index' },
            plugins: {
                legend: { display: false },
                tooltip: {
                    backgroundColor: c.tipBg, titleColor: c.tipTitle, bodyColor: c.tipBody, borderColor: c.tipBorder, borderWidth: 1, padding: 12,
                    filter: item => !item.dataset.label?.startsWith('_'),
                    callbacks: {
                        label: ctx => ctx.dataset.label+': '+fmt(ctx.raw),
                        footer: items => {
                            if (!items.length) return;
                            const d = data[items[0].dataIndex];
                            if (cone && d && d.totalMin !== d.totalAvg) return ['Min: '+fmt(d.totalMin), 'Max: '+fmt(d.totalMax)];
                            return 'Total: '+fmt(items.reduce((s,i) => s+i.raw, 0));
                        }
                    }
                }
            },
            scales: {
                x: { grid: { color: c.grid, drawBorder: false }, ticks: { color: c.text, font: { size: 11 } } },
                y: { stacked: acc?.length > 0, beginAtZero: true, grid: { color: c.grid, drawBorder: false }, ticks: { color: c.text, font: { size: 11 }, callback: fmtAxis } }
            }
        }
    });
};

window.updateProjectionChart = (data, acc) => { window.initProjectionChart(data, acc); };

// Pie chart
window.initPieChart = (accounts, animated = true) => {
    const ctx = document.getElementById('pieCanvas');
    if (!ctx || typeof Chart === 'undefined') return;
    document.getElementById('pie-tooltip')?.remove();
    if (window.pieChart) { clearTimeout(window.pieChart._ttTimer); window.pieChart.destroy(); }
    if (!accounts?.length) return;
    const c = getColors(), bg = getComputedStyle(document.documentElement).getPropertyValue('--background').trim();
    window.pieChart = new Chart(ctx, {
        type: 'doughnut',
        plugins: [tooltipTimeoutPlugin],
        data: { labels: accounts.map(a => a.name), datasets: [{ data: accounts.map(a => a.value), backgroundColor: accounts.map(a => a.color), borderColor: bg, borderWidth: 2, hoverOffset: 0, hoverBorderColor: bg, hoverBorderWidth: 2 }] },
        options: {
            responsive: true, maintainAspectRatio: true, cutout: '65%', animation: animated ? { duration: 400, easing: 'easeOutQuart' } : false,
            plugins: { legend: { display: false }, tooltip: { enabled: false, external: ctx => {
                let t = document.getElementById('pie-tooltip');
                if (!t) { t = document.createElement('div'); t.id = 'pie-tooltip'; t.style.cssText = 'position:fixed;pointer-events:none;padding:8px 12px;border-radius:8px;font-size:13px;z-index:9999;transition:opacity .15s'; document.body.appendChild(t); }
                const tm = ctx.tooltip;
                if (!tm.opacity) { t.style.opacity = 0; return; }
                const cc = getColors(), d = tm.dataPoints[0], total = d.dataset.data.reduce((a,b) => a+b, 0);
                const esc = s => { const d2 = document.createElement('div'); d2.textContent = s; return d2.innerHTML; };
                t.innerHTML = '<strong>'+esc(d.label)+'</strong><br>'+fmt(d.raw)+' ('+(d.raw/total*100).toFixed(1)+'%)';
                const tw = t.offsetWidth || 160, th = t.offsetHeight || 50;
                const rect = ctx.chart.canvas.getBoundingClientRect();
                Object.assign(t.style, { backgroundColor: cc.tipBg, color: cc.tipBody, border: '1px solid '+cc.tipBorder, opacity: 1, left: Math.min(rect.left+tm.caretX+10, window.innerWidth-tw-8)+'px', top: Math.min(rect.top+tm.caretY-10, window.innerHeight-th-8)+'px' });
            } } }
        }
    });
    const leg = document.getElementById('chartLegend');
    if (leg) { const esc = s => { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }; leg.innerHTML = accounts.map(a => '<div class="flex items-center gap-2 text-sm"><span class="w-3 h-3 rounded-full flex-shrink-0" style="background:'+(/^#[0-9a-fA-F]{3,8}$/.test(a.color)?a.color:'#888')+'"></span><span class="text-muted-foreground font-medium">'+esc(a.name)+'</span></div>').join(''); }
    window.pieChartData = accounts;
};

// Theme observer — fade-out, re-render, fade-in on dark/light switch
(function() {
    let last = document.documentElement.classList.contains('dark');
    new MutationObserver(() => {
        const cur = document.documentElement.classList.contains('dark');
        if (cur !== last) {
            last = cur;
            const canvases = [document.getElementById('projectionCanvas'), document.getElementById('pieCanvas')].filter(Boolean);
            canvases.forEach(c => { c.style.transition = 'opacity .2s'; c.style.opacity = '0'; });
            setTimeout(() => {
                window.pieChartData?.length && window.initPieChart(window.pieChartData, false);
                window._projData?.length && window.initProjectionChart(window._projData, window._projAcc);
                canvases.forEach(c => { c.style.opacity = '1'; });
            }, 200);
        }
    }).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
})();
