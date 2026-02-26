package main

const tmplWaterfallV2 = `
{{define "content"}}
<style>
/* ── Waterfall V2 ─────────────────────────────────────────── */
.wf2-toolbar{display:flex;gap:8px;align-items:center;flex-wrap:wrap;padding:4px 0 8px}
.wf2-toolbar select,.wf2-toolbar input[type=text]{background:#161b22;border:1px solid #30363d;color:#c9d1d9;padding:4px 8px;border-radius:4px;font-size:12px;font-family:monospace}
.wf2-toolbar label{font-size:11px;color:#8b949e}
.wf2-toolbar button{background:#1f6feb;border:none;color:#fff;padding:4px 12px;border-radius:4px;cursor:pointer;font-size:12px}
.wf2-toolbar button.sec{background:#21262d;border:1px solid #30363d;color:#c9d1d9}
.wf2-cards{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:8px}
.wf2-card{background:#161b22;border:1px solid #30363d;border-radius:6px;padding:10px 16px;min-width:90px}
.wf2-card-val{font-size:22px;font-weight:700;color:#c9d1d9;font-family:monospace}
.wf2-card-lbl{font-size:10px;color:#8b949e;text-transform:uppercase;letter-spacing:.5px}
.wf2-instance{font-size:11px;color:#58a6ff;margin-bottom:6px}
#wf-outer{position:relative}
#wf-wrap{overflow-y:auto;overflow-x:hidden;max-height:60vh;border:1px solid #30363d;border-radius:4px;background:#0d1117;cursor:crosshair}
#wf-canvas{display:block}
#wf-tooltip{position:fixed;background:#1c2128;border:1px solid #30363d;padding:8px 10px;border-radius:4px;font-size:11px;color:#c9d1d9;pointer-events:none;display:none;z-index:200;max-width:240px;line-height:1.6;box-shadow:0 4px 12px rgba(0,0,0,.5)}
#wf-detail{display:none;flex-direction:column;border:1px solid #30363d;border-radius:4px;max-height:40vh;overflow:hidden;margin-top:8px}
#wf-detail-content{display:flex;flex-direction:column;flex:1;min-height:0;overflow:hidden}
.wf2-dhdr{display:flex;justify-content:space-between;align-items:center;padding:8px 12px;background:#161b22;border-bottom:1px solid #30363d;font-size:12px;flex-wrap:wrap;gap:6px}
.wf2-dhdr button{background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:2px 8px;border-radius:4px;cursor:pointer;font-size:11px}
.wf2-devents{overflow-y:auto;padding:0 8px 8px;flex:1;min-height:0}
.wf2-devents table{width:100%;border-collapse:collapse;font-size:11px}
.wf2-devents th{color:#8b949e;text-align:left;padding:5px 8px;border-bottom:1px solid #30363d;font-weight:600;position:sticky;top:0;background:#0d1117}
.wf2-devents td{padding:3px 8px;border-bottom:1px solid #161b22;vertical-align:top;max-width:400px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.wf2-devents tr:hover td{background:#1c2128}
.err-row td{border-left:2px solid #ef4444}
.wf2-pre{background:#161b22;padding:8px;border-radius:4px;overflow-x:auto;font-size:10px;color:#c9d1d9;margin:8px 0;white-space:pre-wrap;word-break:break-all}
.ev-b{padding:1px 5px;border-radius:3px;font-size:9px;font-family:monospace;white-space:nowrap}
.ev-agent{background:#10b98122;color:#10b981}.ev-api{background:#f59e0b22;color:#f59e0b}
.ev-tool{background:#3b82f622;color:#3b82f6}.ev-bd{background:#ef444422;color:#ef4444}
.ev-sling{background:#f59e0b22;color:#f59e0b}.ev-mail{background:#eab30822;color:#eab308}
.ev-nudge{background:#8b5cf622;color:#8b5cf6}.ev-done{background:#56d36422;color:#56d364}
.ev-prime{background:#3b82f622;color:#3b82f6}.ev-prompt{background:#06b6d422;color:#06b6d4}
.ev-sess{background:#6b728022;color:#6b7280}.ev-inst{background:#8b5cf622;color:#8b5cf6}
.ev-other{background:#30363d;color:#8b949e}
#wf-commmap-body{overflow-x:auto;padding:8px 0}
#wf-commsvg{display:block}
.wf2-legend{display:flex;gap:10px;flex-wrap:wrap;padding:6px 0 2px;font-size:10px;color:#8b949e}
.wf2-leg{display:flex;align-items:center;gap:4px}
.wf2-leg-sq{width:10px;height:10px;border-radius:2px;display:inline-block}
</style>

<div id="wf2-page">

{{if .Instance}}<div class="wf2-instance">instance: <b>{{.Instance}}</b>{{if .TownRoot}} &nbsp;·&nbsp; {{.TownRoot}}{{end}}</div>{{end}}

<div class="wf2-toolbar">
  <label>Rig</label>
  <select id="f-rig"><option value="">All rigs</option></select>
  <label>Role</label>
  <select id="f-role">
    <option value="">All roles</option>
    <option>mayor</option><option>deacon</option><option>witness</option>
    <option>refinery</option><option>polecat</option><option>dog</option>
    <option>boot</option><option>crew</option>
  </select>
  <label>Search</label>
  <input type="text" id="f-search" placeholder="bead ID, agent name…" style="width:160px">
  <button onclick="applyFilters()">Apply</button>
  <button class="sec" onclick="resetFilters()">Reset</button>
  <span style="flex:1"></span>
  <span style="font-size:11px;color:#8b949e" id="wf2-winlabel">window: {{.Window}}</span>
</div>

<div class="wf2-cards">
  <div class="wf2-card"><div class="wf2-card-val" id="wf2-s-runs">—</div><div class="wf2-card-lbl">Runs</div></div>
  <div class="wf2-card"><div class="wf2-card-val" id="wf2-s-rigs">—</div><div class="wf2-card-lbl">Rigs</div></div>
  <div class="wf2-card"><div class="wf2-card-val" id="wf2-s-events">—</div><div class="wf2-card-lbl">Events</div></div>
  <div class="wf2-card"><div class="wf2-card-val" id="wf2-s-beads">—</div><div class="wf2-card-lbl">Beads</div></div>
  <div class="wf2-card"><div class="wf2-card-val" id="wf2-s-cost">—</div><div class="wf2-card-lbl">Cost</div></div>
  <div class="wf2-card"><div class="wf2-card-val" id="wf2-s-dur">—</div><div class="wf2-card-lbl">Total duration</div></div>
</div>

<div class="wf2-legend">
  <span class="wf2-leg"><span class="wf2-leg-sq" style="background:#f59e0b22;border:1px solid #f59e0b"></span>Mayor</span>
  <span class="wf2-leg"><span class="wf2-leg-sq" style="background:#8b5cf622;border:1px solid #8b5cf6"></span>Deacon</span>
  <span class="wf2-leg"><span class="wf2-leg-sq" style="background:#3b82f622;border:1px solid #3b82f6"></span>Witness</span>
  <span class="wf2-leg"><span class="wf2-leg-sq" style="background:#10b98122;border:1px solid #10b981"></span>Refinery</span>
  <span class="wf2-leg"><span class="wf2-leg-sq" style="background:#ef444422;border:1px solid #ef4444"></span>Polecat</span>
  <span class="wf2-leg"><span class="wf2-leg-sq" style="background:#f9731622;border:1px solid #f97316"></span>Dog</span>
  &nbsp;|&nbsp;
  <span class="wf2-leg"><span style="display:inline-block;width:16px;height:2px;background:#f59e0b;vertical-align:middle"></span>API call</span>
  <span class="wf2-leg"><span style="display:inline-block;width:8px;height:8px;background:#10b981;transform:rotate(45deg);vertical-align:middle"></span>Tool ✓</span>
  <span class="wf2-leg"><span style="display:inline-block;width:8px;height:8px;background:#ef4444;transform:rotate(45deg);vertical-align:middle"></span>Tool ✗</span>
  &nbsp;|&nbsp;
  <span class="wf2-leg" style="color:#f59e0b">— sling</span>
  <span class="wf2-leg" style="color:#eab308">— mail</span>
  <span class="wf2-leg" style="color:#8b5cf6">· nudge</span>
  <span class="wf2-leg" style="color:#10b981">— spawn</span>
</div>

<div id="wf-outer">
  <div id="wf-wrap"><canvas id="wf-canvas"></canvas></div>
  <div id="wf-tooltip"></div>
</div>

<div id="wf-detail">
  <div id="wf-detail-content"></div>
</div>

<div class="section" style="margin-top:12px">
  <div class="section-hdr" style="cursor:pointer;user-select:none" onclick="toggleCommMap()">
    Communication Map <span id="wf-cmarrow">▼</span>
  </div>
  <div id="wf-commmap-body">
    <svg id="wf-commsvg"></svg>
  </div>
</div>

</div><!-- #wf2-page -->

<script>
(function(){
// ── Embedded data ─────────────────────────────────────────────────────────────
var DATA = {{.JSONData}};

// ── Constants ─────────────────────────────────────────────────────────────────
var LW   = 200;  // label column width
var ROW  = 50;   // run row height
var RIGH = 28;   // rig header height
var RULH = 36;   // ruler height
var BY   = 6;    // bar top offset within row
var BH   = 22;   // bar height
var TY   = BY + BH + 8;  // tool marker center-y within row
var TR   = 5;    // tool marker half-size

var ROLE_CLR = {
  mayor:'#f59e0b', deacon:'#8b5cf6', witness:'#3b82f6',
  refinery:'#10b981', polecat:'#ef4444', dog:'#f97316',
  boot:'#6b7280', crew:'#06b6d4'
};
function rc(role){ return ROLE_CLR[role]||'#9ca3af'; }

// ── State ─────────────────────────────────────────────────────────────────────
var vStart=0, vEnd=0;
var rows=[];
var collapsed=new Set();
var canvas, ctx, wrap, tip, detPanel, detContent;
var dragging=false, dragX0=0, dragVS0=0, dragVE0=0;
var hovered=null;
// current filter state (applied in-memory on DATA copy)
var filteredData = null;

// ── Init ──────────────────────────────────────────────────────────────────────
function init(){
  canvas     = document.getElementById('wf-canvas');
  ctx        = canvas.getContext('2d');
  wrap       = document.getElementById('wf-wrap');
  tip        = document.getElementById('wf-tooltip');
  detPanel   = document.getElementById('wf-detail');
  detContent = document.getElementById('wf-detail-content');

  filteredData = DATA;

  // Populate rig filter options
  var sel = document.getElementById('f-rig');
  for(var i=0;i<DATA.rigs.length;i++){
    var o=document.createElement('option');
    o.value=o.textContent=DATA.rigs[i].name;
    sel.appendChild(o);
  }

  // Restore filter values from URL
  var params = new URLSearchParams(window.location.search);
  if(params.get('rig'))  document.getElementById('f-rig').value  = params.get('rig');
  if(params.get('role')) document.getElementById('f-role').value = params.get('role');
  if(params.get('q'))    document.getElementById('f-search').value = params.get('q');

  // Apply filters if any are set
  applyFiltersLocal();
  computeTimeRange();
  updateSummary();
  rebuildRows();
  buildCommMap();

  canvas.addEventListener('mousemove', onMove);
  canvas.addEventListener('mouseleave', onLeave);
  canvas.addEventListener('click', onClick);
  canvas.addEventListener('wheel', onWheel, {passive:false});
  canvas.addEventListener('mousedown', onDown);
  window.addEventListener('mousemove', onWinMove);
  window.addEventListener('mouseup', onUp);
  window.addEventListener('resize', onResize);

  requestAnimationFrame(draw);
}

function applyFiltersLocal(){
  var rigF  = document.getElementById('f-rig').value;
  var roleF = document.getElementById('f-role').value;
  var qF    = document.getElementById('f-search').value.toLowerCase();

  // Deep-copy rigs and filter
  var rigs = DATA.rigs.map(function(rig){
    if(rigF && rig.name !== rigF) return null;
    var runs = rig.runs.filter(function(run){
      if(roleF && run.role !== roleF) return false;
      if(qF){
        var hay = (run.run_id+run.agent_name+run.role+run.session_id+run.rig).toLowerCase();
        if(hay.indexOf(qF)<0) return false;
      }
      return true;
    });
    if(runs.length===0) return null;
    return {name:rig.name, collapsed:rig.collapsed, runs:runs};
  }).filter(Boolean);

  filteredData = {
    instance:       DATA.instance,
    town_root:      DATA.town_root,
    window:         DATA.window,
    summary:        DATA.summary,
    rigs:           rigs,
    communications: DATA.communications,
    beads:          DATA.beads
  };
}

function computeTimeRange(){
  var mn=Infinity, mx=-Infinity;
  for(var i=0;i<filteredData.rigs.length;i++){
    var rig=filteredData.rigs[i];
    for(var j=0;j<rig.runs.length;j++){
      var run=rig.runs[j];
      var st=new Date(run.started_at).getTime();
      var et=run.ended_at?new Date(run.ended_at).getTime():Date.now();
      if(st<mn) mn=st;
      if(et>mx) mx=et;
    }
  }
  if(!isFinite(mn)){ mn=Date.now()-3600000; mx=Date.now(); }
  var pad=(mx-mn)*0.04||5000;
  vStart=mn-pad; vEnd=mx+pad;
}

function rebuildRows(){
  rows=[];
  for(var i=0;i<filteredData.rigs.length;i++){
    var rig=filteredData.rigs[i];
    rows.push({type:'rig', name:rig.name, rig:rig});
    if(!collapsed.has(rig.name)){
      for(var j=0;j<rig.runs.length;j++){
        rows.push({type:'run', run:rig.runs[j], rigName:rig.name});
      }
    }
  }
  var totalH=RULH;
  for(var k=0;k<rows.length;k++) totalH+=(rows[k].type==='rig'?RIGH:ROW);
  var W=wrap.clientWidth||900;
  canvas.width  = W;
  canvas.height = Math.max(300, totalH+20);
  ctx = canvas.getContext('2d');
}

function onResize(){ rebuildRows(); draw(); }

// ── Drawing ───────────────────────────────────────────────────────────────────
function draw(){
  var W=canvas.width, H=canvas.height;
  ctx.fillStyle='#0d1117';
  ctx.fillRect(0,0,W,H);

  drawGrid(W,H);

  var y=RULH;
  for(var i=0;i<rows.length;i++){
    var row=rows[i];
    if(row.type==='rig'){drawRigRow(row,y,W); y+=RIGH;}
    else{drawRunRow(row,y,W); y+=ROW;}
  }

  drawComms(W);
  drawRuler(W);

  // Label column right border
  ctx.strokeStyle='#30363d'; ctx.lineWidth=1;
  ctx.beginPath(); ctx.moveTo(LW,0); ctx.lineTo(LW,H); ctx.stroke();
}

function niceInterval(rangeMs,pixW){
  var ivs=[500,1000,2000,5000,10000,15000,30000,60000,120000,300000,600000,1800000,3600000,7200000];
  for(var i=0;i<ivs.length;i++){
    if(pixW/(rangeMs/ivs[i])>=70) return ivs[i];
  }
  return ivs[ivs.length-1];
}

function toX(t,W){ var a=W-LW; return LW+((t-vStart)/(vEnd-vStart))*a; }
function xToT(x,W){ var a=W-LW; return vStart+((x-LW)/a)*(vEnd-vStart); }

function rulerLabel(t){
  var d=new Date(t), r=vEnd-vStart;
  var hh=d.getHours().toString().padStart(2,'0');
  var mm=d.getMinutes().toString().padStart(2,'0');
  var ss=d.getSeconds().toString().padStart(2,'0');
  if(r<90000) return hh+':'+mm+':'+ss;
  return hh+':'+mm;
}

function drawGrid(W,H){
  var iv=niceInterval(vEnd-vStart,W-LW);
  var first=Math.ceil(vStart/iv)*iv;
  ctx.strokeStyle='#1c2128'; ctx.lineWidth=0.5;
  for(var t=first;t<=vEnd;t+=iv){
    var x=toX(t,W);
    ctx.beginPath(); ctx.moveTo(x,0); ctx.lineTo(x,H); ctx.stroke();
  }
}

function drawRuler(W){
  ctx.fillStyle='#161b22'; ctx.fillRect(0,0,W,RULH);
  ctx.fillStyle='#0d1117'; ctx.fillRect(0,0,LW,RULH);
  ctx.strokeStyle='#30363d'; ctx.lineWidth=1;
  ctx.beginPath(); ctx.moveTo(0,RULH); ctx.lineTo(W,RULH); ctx.stroke();

  ctx.fillStyle='#8b949e'; ctx.font='10px monospace'; ctx.textAlign='left';
  ctx.fillText('Agent / Run', 8, RULH-10);

  var iv=niceInterval(vEnd-vStart,W-LW);
  var first=Math.ceil(vStart/iv)*iv;
  ctx.textAlign='center'; ctx.fillStyle='#8b949e';
  for(var t=first;t<=vEnd;t+=iv){
    var x=toX(t,W);
    ctx.fillText(rulerLabel(t), x, 16);
    ctx.strokeStyle='#30363d'; ctx.lineWidth=1;
    ctx.beginPath(); ctx.moveTo(x,RULH-5); ctx.lineTo(x,RULH); ctx.stroke();
  }
}

function drawRigRow(row,y,W){
  ctx.fillStyle='#0d1117'; ctx.fillRect(0,y,W,RIGH);
  ctx.strokeStyle='#30363d'; ctx.lineWidth=1;
  ctx.beginPath(); ctx.moveTo(0,y+RIGH-1); ctx.lineTo(W,y+RIGH-1); ctx.stroke();
  var arrow=collapsed.has(row.name)?'▶':'▼';
  ctx.fillStyle='#58a6ff'; ctx.font='bold 11px monospace'; ctx.textAlign='left';
  ctx.fillText(arrow+' '+row.name, 8, y+18);
  // run count dim
  if(!collapsed.has(row.name)){
    ctx.fillStyle='#30363d'; ctx.font='10px monospace';
    ctx.fillText(row.rig.runs.length+' runs', LW+8, y+18);
  }
}

function drawRunRow(row,y,W){
  var run=row.run;
  var isHov=(hovered&&hovered.run&&hovered.run.run_id===run.run_id);
  ctx.fillStyle=isHov?'#1c2128':'#0d1117';
  ctx.fillRect(0,y,W,ROW);
  ctx.strokeStyle='#21262d'; ctx.lineWidth=1;
  ctx.beginPath(); ctx.moveTo(0,y+ROW-1); ctx.lineTo(W,y+ROW-1); ctx.stroke();

  // Label
  var color=rc(run.role);
  ctx.fillStyle='#c9d1d9'; ctx.font='11px monospace'; ctx.textAlign='left';
  var label=(run.agent_name||run.role||run.session_id||run.run_id.substring(0,8));
  ctx.fillText(label, 8, y+15);

  ctx.fillStyle=color+'22'; ctx.fillRect(8,y+22,56,13);
  ctx.fillStyle=color; ctx.font='9px monospace'; ctx.textAlign='center';
  ctx.fillText((run.role||'?').substring(0,7), 36, y+32);

  if(run.rig){
    ctx.fillStyle='#8b949e'; ctx.font='9px monospace'; ctx.textAlign='left';
    ctx.fillText(run.rig, 68, y+32);
  }
  if(run.cost>0){
    ctx.fillStyle='#56d364'; ctx.font='9px monospace'; ctx.textAlign='right';
    ctx.fillText('$'+run.cost.toFixed(3), LW-4, y+32);
  }

  // Session bar
  var st=new Date(run.started_at).getTime();
  var et=run.ended_at?new Date(run.ended_at).getTime():vEnd;
  if(st<vEnd && et>vStart){
    var x1=Math.max(LW, toX(st,W));
    var x2=Math.min(W,  toX(et,W));
    var bw=Math.max(3, x2-x1);
    ctx.fillStyle=color+'25'; ctx.fillRect(x1,y+BY,bw,BH);
    ctx.strokeStyle=color+'aa'; ctx.lineWidth=1.5;
    ctx.strokeRect(x1+.5,y+BY+.5,bw-1,BH-1);
    ctx.lineWidth=1;
    if(run.running){
      ctx.fillStyle=color;
      ctx.beginPath();
      ctx.arc(Math.min(W-6,x2-3),y+BY+BH/2,3,0,2*Math.PI);
      ctx.fill();
    }
  }

  // Events
  var evs=run.events||[];
  for(var i=0;i<evs.length;i++){
    var ev=evs[i];
    var et2=new Date(ev.timestamp).getTime();
    if(et2<vStart||et2>vEnd) continue;
    var ex=toX(et2,W);
    if(ex<LW||ex>W) continue;

    if(ev.body==='claude_code.api_request'){
      ctx.strokeStyle='#f59e0b'; ctx.lineWidth=1.5; ctx.globalAlpha=0.65;
      ctx.beginPath(); ctx.moveTo(ex,y+BY+2); ctx.lineTo(ex,y+BY+BH-2); ctx.stroke();
      ctx.globalAlpha=1; ctx.lineWidth=1;
    } else if(ev.body==='claude_code.tool_result'){
      var ok=(ev.attrs&&ev.attrs.success!=='false');
      ctx.fillStyle=ok?'#10b981':'#ef4444';
      ctx.beginPath();
      ctx.moveTo(ex,     y+TY-TR);
      ctx.lineTo(ex+TR,  y+TY);
      ctx.lineTo(ex,     y+TY+TR);
      ctx.lineTo(ex-TR,  y+TY);
      ctx.closePath(); ctx.fill();
    }
  }
}

function drawComms(W){
  var runY={};
  var y=RULH;
  for(var i=0;i<rows.length;i++){
    var row=rows[i];
    if(row.type==='rig') y+=RIGH;
    else{
      runY[row.run.run_id]=y+BY+BH/2;
      if(row.run.session_id) runY[row.run.session_id]=y+BY+BH/2;
      y+=ROW;
    }
  }
  var CC={sling:'#f59e0b',mail:'#eab308',nudge:'#8b5cf6',spawn:'#10b981',done:'#3b82f6'};
  var comms=filteredData.communications||[];
  ctx.lineWidth=1.5;
  for(var k=0;k<comms.length;k++){
    var c=comms[k];
    var ct=new Date(c.time).getTime();
    if(ct<vStart||ct>vEnd) continue;
    var cx=toX(ct,W);
    if(cx<LW||cx>W) continue;
    var color=CC[c.type]||'#9ca3af';
    var fy=runY[c.from], ty2=runY[c.to];
    ctx.strokeStyle=color; ctx.fillStyle=color; ctx.globalAlpha=0.75;
    if(fy!==undefined&&ty2!==undefined&&fy!==ty2){
      var cp=Math.abs(ty2-fy)*0.35;
      ctx.beginPath();
      ctx.moveTo(cx,fy);
      ctx.bezierCurveTo(cx+cp,fy,cx+cp,ty2,cx,ty2);
      ctx.stroke();
      var dir=ty2>fy?1:-1;
      ctx.beginPath();
      ctx.moveTo(cx,ty2);
      ctx.lineTo(cx-4,ty2-6*dir);
      ctx.lineTo(cx+4,ty2-6*dir);
      ctx.closePath(); ctx.fill();
    } else if(fy!==undefined){
      ctx.beginPath();
      ctx.arc(cx,fy,4,0,2*Math.PI);
      ctx.stroke();
    }
    ctx.globalAlpha=1;
    // micro label
    if(fy!==undefined){
      ctx.fillStyle=color; ctx.font='8px monospace'; ctx.textAlign='left';
      ctx.fillText(c.type.substring(0,4),cx+5,(fy!==undefined?fy:0)-2);
    }
  }
  ctx.lineWidth=1;
}

// ── Hit test ──────────────────────────────────────────────────────────────────
function hitTest(x,y){
  var ry=RULH;
  for(var i=0;i<rows.length;i++){
    var row=rows[i];
    var h=row.type==='rig'?RIGH:ROW;
    if(y>=ry&&y<ry+h){
      if(row.type==='rig') return{type:'rig',rigName:row.name};
      if(x>=LW){
        var run=row.run;
        var evs=run.events||[];
        // tools first
        for(var j=0;j<evs.length;j++){
          var ev=evs[j];
          if(ev.body!=='claude_code.tool_result') continue;
          var ex=toX(new Date(ev.timestamp).getTime(),canvas.width);
          if(Math.abs(x-ex)<=TR+3&&y>=ry+TY-TR-3&&y<=ry+TY+TR+3)
            return{type:'tool',ev:ev,run:run};
        }
        // api ticks
        for(var j=0;j<evs.length;j++){
          var ev=evs[j];
          if(ev.body!=='claude_code.api_request') continue;
          var ex=toX(new Date(ev.timestamp).getTime(),canvas.width);
          if(Math.abs(x-ex)<=5&&y>=ry+BY&&y<=ry+BY+BH)
            return{type:'api',ev:ev,run:run};
        }
        // session bar
        var st=new Date(run.started_at).getTime();
        var et=run.ended_at?new Date(run.ended_at).getTime():vEnd;
        var t=xToT(x,canvas.width);
        if(t>=st&&t<=et&&y>=ry+BY&&y<=ry+BY+BH)
          return{type:'session',run:run};
      }
      if(x<LW) return{type:'label',run:row.run};
      return null;
    }
    ry+=h;
  }
  return null;
}

// ── Events ────────────────────────────────────────────────────────────────────
function onMove(e){
  if(dragging) return;
  var r=canvas.getBoundingClientRect();
  var x=e.clientX-r.left, y=e.clientY-r.top;
  var item=hitTest(x,y);
  if(item){ canvas.style.cursor='pointer'; showTip(e,item); hovered=item; }
  else     { canvas.style.cursor='crosshair'; hideTip(); hovered=null; }
  draw();
}
function onLeave(){ hideTip(); hovered=null; draw(); }

function onClick(e){
  var r=canvas.getBoundingClientRect();
  var item=hitTest(e.clientX-r.left, e.clientY-r.top);
  if(!item) return;
  if(item.type==='rig'){
    collapsed.has(item.rigName)?collapsed.delete(item.rigName):collapsed.add(item.rigName);
    rebuildRows(); draw(); return;
  }
  if(item.type==='session'||item.type==='label') showDetail(item.run);
  else if(item.type==='api')  showDetailAPI(item.ev, item.run);
  else if(item.type==='tool') showDetailTool(item.ev, item.run);
}

function onWheel(e){
  e.preventDefault();
  var r=canvas.getBoundingClientRect();
  var x=e.clientX-r.left;
  if(x<LW) return;
  var t=xToT(x,canvas.width);
  var factor=e.deltaY>0?1.25:0.8;
  var range=(vEnd-vStart)*factor;
  var ratio=(t-vStart)/(vEnd-vStart);
  vStart=t-ratio*range; vEnd=t+(1-ratio)*range;
  draw();
}
function onDown(e){
  var r=canvas.getBoundingClientRect();
  if(e.clientX-r.left<LW) return;
  dragging=true; dragX0=e.clientX; dragVS0=vStart; dragVE0=vEnd;
  canvas.style.cursor='grabbing';
}
function onWinMove(e){
  if(!dragging) return;
  var dx=e.clientX-dragX0;
  var avail=canvas.width-LW;
  var dT=dx/avail*(dragVE0-dragVS0);
  vStart=dragVS0-dT; vEnd=dragVE0-dT;
  draw();
}
function onUp(){ dragging=false; canvas.style.cursor='crosshair'; }

// ── Tooltip ───────────────────────────────────────────────────────────────────
function showTip(e,item){
  var html='';
  if(item.type==='session'||item.type==='label'){
    var r=item.run;
    var dur=r.duration_ms?fmtMs(r.duration_ms):(r.running?'running':'—');
    html='<b>'+(r.agent_name||r.role)+'</b><br>'
        +'Role: '+(r.role||'—')+' &nbsp; Rig: '+(r.rig||'town')+'<br>'
        +'run.id: '+(r.run_id?r.run_id.substring(0,12)+'…':'—')+'<br>'
        +'Duration: '+dur+'<br>'
        +(r.cost>0?'Cost: $'+r.cost.toFixed(4)+'<br>':'')
        +'Events: '+(r.events||[]).length;
  } else if(item.type==='api'){
    var a=item.ev.attrs||{};
    html='<b>API call</b><br>'+
         'Model: '+(a.model||'—')+'<br>'+
         'In: '+(a.input_tokens||0)+' &nbsp; Out: '+(a.output_tokens||0)+'<br>'+
         'Cache: '+(a.cache_read_tokens||0)+'<br>'+
         'Cost: $'+parseFloat(a.cost_usd||0).toFixed(5)+'<br>'+
         'Dur: '+fmtMs(parseFloat(a.duration_ms||0));
  } else if(item.type==='tool'){
    var a=item.ev.attrs||{};
    html='<b>'+(a.tool_name||'Tool')+'</b><br>'+
         'Dur: '+fmtMs(parseFloat(a.duration_ms||0))+'<br>'+
         (a.success!=='false'?'<span style="color:#56d364">✓ success</span>':'<span style="color:#ef4444">✗ failed</span>');
  } else if(item.type==='rig'){
    html='<b>Rig: '+item.rigName+'</b><br>Click to collapse/expand';
  }
  if(!html) return;
  tip.innerHTML=html;
  tip.style.display='block';
  tip.style.left=(e.clientX+14)+'px';
  tip.style.top=(e.clientY-10)+'px';
  // Clamp to viewport right edge
  var tw=tip.offsetWidth;
  if(e.clientX+14+tw>window.innerWidth) tip.style.left=(e.clientX-tw-14)+'px';
}
function hideTip(){ tip.style.display='none'; }

// ── Detail panel ──────────────────────────────────────────────────────────────
function evBadge(body){
  var m={
    'agent.event':'ev-agent','claude_code.api_request':'ev-api',
    'claude_code.tool_result':'ev-tool','bd.call':'ev-bd',
    'sling':'ev-sling','mail':'ev-mail','nudge':'ev-nudge','done':'ev-done',
    'prime':'ev-prime','prompt.send':'ev-prompt',
    'session.start':'ev-sess','session.stop':'ev-sess',
    'agent.instantiate':'ev-inst'
  };
  return '<span class="ev-b '+(m[body]||'ev-other')+'">'+esc(body)+'</span>';
}

function evDetail(ev){
  var a=ev.attrs||{};
  if(ev.body==='agent.event')
    return esc((a.content||'').substring(0,140));
  if(ev.body==='bd.call')
    return esc(((a.subcommand||'')+' '+(a.args||'')).substring(0,100));
  if(ev.body==='claude_code.api_request')
    return esc((a.model||'')+' in:'+(a.input_tokens||0)+' out:'+(a.output_tokens||0)+' $'+parseFloat(a.cost_usd||0).toFixed(4));
  if(ev.body==='claude_code.tool_result')
    return esc((a.tool_name||'')+' '+(a.success!=='false'?'✓':'✗')+' '+fmtMs(parseFloat(a.duration_ms||0)));
  if(ev.body==='mail')
    return esc(((a.operation||'')+' '+ato(a,'msg.from')+' → '+ato(a,'msg.to')+': '+ato(a,'msg.subject')).substring(0,100));
  if(ev.body==='prompt.send')
    return esc((a.keys_len||0)+' bytes'+(a.keys?' — '+a.keys.substring(0,80):''));
  if(ev.body==='done') return esc(a.exit_type||'');
  if(ev.body==='prime') return esc((a.formula||'').substring(0,80));
  var keys=Object.keys(a).filter(function(k){return k!=='run.id'&&!k.startsWith('gt.');}).slice(0,4);
  return esc(keys.map(function(k){return k+'='+a[k];}).join(' '));
}

function ato(a,k){ return a[k]||''; }

function showDetail(run){
  var dur=run.duration_ms?fmtMs(run.duration_ms):(run.running?'running':'—');
  var html='<div class="wf2-dhdr">'
    +'<span><b>'+(run.agent_name||run.role)+'</b>'
    +' &nbsp;|&nbsp; role: '+esc(run.role||'—')
    +' &nbsp;|&nbsp; rig: '+esc(run.rig||'town')+'</span>'
    +'<span>started: '+fmtTime(run.started_at)
    +' &nbsp; dur: '+dur
    +' &nbsp; cost: $'+run.cost.toFixed(4)+'</span>'
    +'<button onclick="closeDetail()">✕</button>'
    +'</div>'
    +'<div class="wf2-devents"><table>'
    +'<thead><tr><th>Time</th><th>Event</th><th>Detail</th></tr></thead><tbody>';
  var evs=run.events||[];
  for(var i=0;i<evs.length;i++){
    var ev=evs[i];
    var d=new Date(ev.timestamp);
    var ts=d.getHours().toString().padStart(2,'0')+':'+d.getMinutes().toString().padStart(2,'0')+':'+d.getSeconds().toString().padStart(2,'0');
    html+='<tr class="'+(ev.severity==='error'?'err-row':'')+'"><td class="mono dim">'+ts+'</td>'
         +'<td>'+evBadge(ev.body)+'</td>'
         +'<td class="mono">'+evDetail(ev)+'</td></tr>';
  }
  html+='</tbody></table></div>';
  detContent.innerHTML=html;
  detPanel.style.display='flex';
}

function showDetailAPI(ev, run){
  var a=ev.attrs||{};
  detContent.innerHTML='<div class="wf2-dhdr"><span><b>API Call</b> — '+(run.agent_name||run.role)+'</span>'
    +'<button onclick="closeDetail()">✕</button></div>'
    +'<div class="wf2-devents"><table>'
    +'<tr><th>Model</th><td>'+esc(a.model||'—')+'</td></tr>'
    +'<tr><th>Input tokens</th><td>'+(a.input_tokens||0)+'</td></tr>'
    +'<tr><th>Output tokens</th><td>'+(a.output_tokens||0)+'</td></tr>'
    +'<tr><th>Cache read</th><td>'+(a.cache_read_tokens||0)+'</td></tr>'
    +'<tr><th>Cost</th><td>$'+parseFloat(a.cost_usd||0).toFixed(6)+'</td></tr>'
    +'<tr><th>Duration</th><td>'+fmtMs(parseFloat(a.duration_ms||0))+'</td></tr>'
    +'<tr><th>Session ID</th><td class="mono">'+esc(a['session.id']||'—')+'</td></tr>'
    +'</table></div>';
  detPanel.style.display='flex';
}

function showDetailTool(ev, run){
  var a=ev.attrs||{};
  detContent.innerHTML='<div class="wf2-dhdr"><span><b>'+(a.tool_name||'Tool')+'</b> — '+(run.agent_name||run.role)+'</span>'
    +'<button onclick="closeDetail()">✕</button></div>'
    +'<div class="wf2-devents"><table>'
    +'<tr><th>Tool</th><td>'+esc(a.tool_name||'—')+'</td></tr>'
    +'<tr><th>Duration</th><td>'+fmtMs(parseFloat(a.duration_ms||0))+'</td></tr>'
    +'<tr><th>Success</th><td>'+(a.success!=='false'?'<span style="color:#56d364">✓ yes</span>':'<span style="color:#ef4444">✗ no</span>')+'</td></tr>'
    +'</table>'
    +(a.tool_parameters?'<pre class="wf2-pre">'+esc(a.tool_parameters.substring(0,600))+'</pre>':'')
    +'</div>';
  detPanel.style.display='flex';
}

function closeDetail(){ detPanel.style.display='none'; }
window.closeDetail=closeDetail;

// ── Summary ───────────────────────────────────────────────────────────────────
function updateSummary(){
  var s=DATA.summary;
  set('wf2-s-runs',  s.run_count||0);
  set('wf2-s-rigs',  s.rig_count||0);
  set('wf2-s-events',s.event_count||0);
  set('wf2-s-beads', s.bead_count||0);
  set('wf2-s-cost',  '$'+((s.total_cost||0).toFixed(4)));
  set('wf2-s-dur',   s.total_duration||'—');
}
function set(id,v){ var el=document.getElementById(id); if(el) el.textContent=v; }

// ── Communication map (SVG) ───────────────────────────────────────────────────
function buildCommMap(){
  var svg=document.getElementById('wf-commsvg'); if(!svg) return;
  var allRuns=[];
  for(var i=0;i<DATA.rigs.length;i++)
    for(var j=0;j<DATA.rigs[i].runs.length;j++)
      allRuns.push(DATA.rigs[i].runs[j]);
  if(!allRuns.length){
    svg.innerHTML='<text x="8" y="20" fill="#8b949e" font-size="12">No runs in window</text>';
    return;
  }
  var W=svg.parentElement.clientWidth||800;
  var cols=Math.min(7,allRuns.length);
  var rows2=Math.ceil(allRuns.length/cols);
  var H=Math.max(120,rows2*90+40);
  svg.setAttribute('viewBox','0 0 '+W+' '+H);
  svg.setAttribute('width',W); svg.setAttribute('height',H);

  var nr=26;
  var nodePos={};
  for(var i=0;i<allRuns.length;i++){
    var col=i%cols;
    var row=Math.floor(i/cols);
    var gapX=cols>1?(W-80)/(cols-1):0;
    var x=40+col*gapX;
    var y=50+row*90;
    nodePos[allRuns[i].run_id]={x:x,y:y};
    if(allRuns[i].session_id) nodePos[allRuns[i].session_id]={x:x,y:y};
  }

  var edges={};
  var comms=DATA.communications||[];
  for(var k=0;k<comms.length;k++){
    var c=comms[k]; if(!c.to) continue;
    var key=c.from+'|'+c.to;
    if(!edges[key]) edges[key]={from:c.from,to:c.to,count:0,type:c.type};
    edges[key].count++;
  }
  var CC={sling:'#f59e0b',mail:'#eab308',nudge:'#8b5cf6',spawn:'#10b981',done:'#3b82f6'};

  var s='<defs><marker id="ca" markerWidth="7" markerHeight="5" refX="7" refY="2.5" orient="auto">'
       +'<polygon points="0 0,7 2.5,0 5" fill="#8b949e"/></marker></defs>';

  for(var key in edges){
    var e2=edges[key];
    var fp=nodePos[e2.from], tp=nodePos[e2.to];
    if(!fp||!tp) continue;
    var color=CC[e2.type]||'#9ca3af';
    var lw=Math.min(3,1+e2.count*0.5);
    if(fp===tp){
      s+='<circle cx="'+(fp.x+nr+5)+'" cy="'+(fp.y-nr/2)+'" r="10" fill="none" stroke="'+color+'" stroke-width="'+lw+'" stroke-opacity="0.6"/>';
    } else {
      var mx=(fp.x+tp.x)/2, my=(fp.y+tp.y)/2;
      s+='<line x1="'+fp.x+'" y1="'+fp.y+'" x2="'+tp.x+'" y2="'+tp.y
         +'" stroke="'+color+'" stroke-width="'+lw+'" stroke-opacity="0.6" marker-end="url(#ca)"/>';
      s+='<text x="'+mx+'" y="'+(my-3)+'" fill="'+color+'" font-size="8" text-anchor="middle" opacity="0.8">'+e2.count+'</text>';
    }
  }
  for(var i=0;i<allRuns.length;i++){
    var run=allRuns[i], p=nodePos[run.run_id]; if(!p) continue;
    var color=rc(run.role);
    s+='<circle cx="'+p.x+'" cy="'+p.y+'" r="'+nr+'" fill="'+color+'22" stroke="'+color+'" stroke-width="1.5"/>';
    s+='<text x="'+p.x+'" y="'+(p.y-5)+'" fill="'+color+'" font-size="10" font-weight="bold" text-anchor="middle">'
      +esc((run.role||'?').substring(0,6))+'</text>';
    s+='<text x="'+p.x+'" y="'+(p.y+9)+'" fill="#8b949e" font-size="8" text-anchor="middle">'
      +esc((run.agent_name||'').substring(0,10))+'</text>';
    if(run.rig){
      s+='<text x="'+p.x+'" y="'+(p.y+20)+'" fill="#58a6ff" font-size="7" text-anchor="middle">'+esc(run.rig)+'</text>';
    }
  }
  svg.innerHTML=s;
}

// ── Filters ───────────────────────────────────────────────────────────────────
function applyFilters(){
  var params=new URLSearchParams(window.location.search);
  var rig=document.getElementById('f-rig').value;
  var role=document.getElementById('f-role').value;
  var q=document.getElementById('f-search').value;
  if(rig)  params.set('rig',rig);   else params.delete('rig');
  if(role) params.set('role',role); else params.delete('role');
  if(q)    params.set('q',q);       else params.delete('q');
  window.history.replaceState(null,'','?'+params.toString());
  applyFiltersLocal();
  computeTimeRange();
  rebuildRows();
  buildCommMap();
  draw();
}
window.applyFilters=applyFilters;

function resetFilters(){
  var params=new URLSearchParams(window.location.search);
  params.delete('rig'); params.delete('role'); params.delete('q');
  window.location.href='?'+params.toString();
}
window.resetFilters=resetFilters;

// ── Comm map toggle ───────────────────────────────────────────────────────────
function toggleCommMap(){
  var body=document.getElementById('wf-commmap-body');
  var arrow=document.getElementById('wf-cmarrow');
  if(body.style.display==='none'){body.style.display='block';arrow.textContent='▼';}
  else{body.style.display='none';arrow.textContent='▶';}
}
window.toggleCommMap=toggleCommMap;

// ── Utilities ─────────────────────────────────────────────────────────────────
function fmtMs(ms){
  if(ms<1000) return Math.round(ms)+'ms';
  if(ms<60000) return (ms/1000).toFixed(1)+'s';
  return Math.floor(ms/60000)+'m'+Math.floor((ms%60000)/1000)+'s';
}
function fmtTime(ts){
  var d=new Date(ts);
  return d.getHours().toString().padStart(2,'0')+':'
        +d.getMinutes().toString().padStart(2,'0')+':'
        +d.getSeconds().toString().padStart(2,'0');
}
function esc(s){
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

window.addEventListener('load', init);
})();
</script>
{{end}}
`
