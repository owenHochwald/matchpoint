import React from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  Bolt,
  Crown,
  Gauge,
  Gem,
  Play,
  Shield,
  Swords,
  Trophy,
  Wifi,
  WifiOff,
  Zap,
} from "lucide-react";
import "./style.css";

type Frame = {
  ts: number;
  queueDepths: number[];
  matchesLastTick: number;
  eommAccuracy: number;
  allocBytesHeap: number;
  churnAlerts: number;
  coreTicks: number;
  drainedTickets: number;
  candidateQueries: number;
  emptyQueries: number;
  totalDrained: number;
  totalCandidates: number;
  totalMatches: number;
  totalEmptyQueries: number;
  tickDurationNanos: number;
  redisLatencyNanos: number;
  redisStatus: number;
  overruns: number;
  skippedTicks: number;
  simDrops: number;
};

type QueueAck = {
  status: string;
  playerId: number;
  shard: number;
  depth: number;
  message?: string;
};

const emptyFrame: Frame = {
  ts: 0,
  queueDepths: [0, 0, 0, 0, 0],
  matchesLastTick: 0,
  eommAccuracy: 0,
  allocBytesHeap: 0,
  churnAlerts: 0,
  coreTicks: 0,
  drainedTickets: 0,
  candidateQueries: 0,
  emptyQueries: 0,
  totalDrained: 0,
  totalCandidates: 0,
  totalMatches: 0,
  totalEmptyQueries: 0,
  tickDurationNanos: 0,
  redisLatencyNanos: 0,
  redisStatus: 0,
  overruns: 0,
  skippedTicks: 0,
  simDrops: 0,
};

const arenaNames = ["Training", "River", "Builder", "Royal", "Legend", "Champion"];
const poolNames = ["Mainstream", "Loser", "Retention", "Monetize"];
const poolGuides = [
  {
    tag: 0,
    name: "Mainstream",
    short: "Default ranked flow",
    detail: "Uses trophy lanes first. This is the baseline pool for normal queue health and fair-feeling matches.",
  },
  {
    tag: 1,
    name: "Loser",
    short: "Losing-streak rescue",
    detail: "Groups players on rough streaks so the system can watch tilt pressure without mixing every player into one lane.",
  },
  {
    tag: 2,
    name: "Retention",
    short: "Churn-risk protection",
    detail: "Routes fragile sessions into a pool where match quality matters more because the next match may decide whether they leave.",
  },
  {
    tag: 3,
    name: "Monetize",
    short: "High-spend sensitivity",
    detail: "Separates monetization-sensitive traffic so spend signals can be measured without hiding queue pressure in the main pool.",
  },
];

function useTelemetry() {
  const [frame, setFrame] = React.useState<Frame>(emptyFrame);
  const [connected, setConnected] = React.useState(false);
  const [history, setHistory] = React.useState<Frame[]>([]);

  React.useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${protocol}://${window.location.host}/telemetry`);
    socket.onopen = () => setConnected(true);
    socket.onclose = () => setConnected(false);
    socket.onerror = () => setConnected(false);
    socket.onmessage = (event) => {
      const next = JSON.parse(event.data) as Frame;
      setFrame(next);
      setHistory((items) => [...items.slice(-47), next]);
    };
    return () => socket.close();
  }, []);

  return { frame, connected, history };
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(Math.round(value));
}

function percent(value: number) {
  return `${Math.round(value * 100)}%`;
}

function millisFromNanos(value: number) {
  return value / 1_000_000;
}

function StatTile({
  label,
  value,
  detail,
  tone,
  icon,
}: {
  label: string;
  value: string;
  detail: string;
  tone: "gold" | "sky" | "pink" | "emerald";
  icon: React.ReactNode;
}) {
  return (
    <section className={`stat-tile stat-${tone}`}>
      <div className="flex items-center justify-between gap-3">
        <span className="stat-icon">{icon}</span>
        <span className="stat-label">{label}</span>
      </div>
      <p className="mt-3 text-3xl font-black tracking-normal text-white sm:text-4xl">{value}</p>
      <p className="mt-1 text-sm font-bold text-white/70">{detail}</p>
    </section>
  );
}

function Slider({
  label,
  value,
  min,
  max,
  step = 1,
  onChange,
  suffix = "",
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step?: number;
  onChange: (value: number) => void;
  suffix?: string;
}) {
  return (
    <label className="control-row">
      <span>{label}</span>
      <strong>
        {formatNumber(value)}
        {suffix}
      </strong>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  );
}

function Sparkline({ values, color, label }: { values: number[]; color: string; label: string }) {
  const max = Math.max(1, ...values);
  const width = 360;
  const height = 110;
  const points = values.length
    ? values
        .map((value, index) => {
          const x = values.length === 1 ? 0 : (index / (values.length - 1)) * width;
          const y = height - (value / max) * (height - 12) - 6;
          return `${x},${y}`;
        })
        .join(" ")
    : `0,${height} ${width},${height}`;

  return (
    <div className="spark-panel">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-black uppercase text-white/80">{label}</span>
        <span className="rounded-full bg-white/15 px-3 py-1 text-xs font-black text-white">{formatNumber(max)}</span>
      </div>
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={label} className="h-28 w-full overflow-visible">
        <defs>
          <linearGradient id={`fill-${label}`} x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity="0.42" />
            <stop offset="100%" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        <polyline
          points={`0,${height} ${points} ${width},${height}`}
          fill={`url(#fill-${label})`}
          stroke="none"
        />
        <polyline points={points} fill="none" stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth="7" />
      </svg>
    </div>
  );
}

function QueueArena({ depths }: { depths: number[] }) {
  const max = Math.max(1, ...depths);
  return (
    <section className="arena-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Live Queue</span>
          <h2>Battle Lanes</h2>
        </div>
        <Shield className="h-7 w-7 text-amber-200" />
      </div>
      <div className="mt-5 grid gap-3">
        {depths.map((depth, index) => (
          <div key={index} className="lane-row">
            <div className="flex min-w-0 items-center gap-3">
              <span className="lane-badge">{index + 1}</span>
              <span className="truncate text-sm font-black text-white">{arenaNames[index] ?? `Arena ${index + 1}`}</span>
            </div>
            <div className="lane-track">
              <div className="lane-fill" style={{ width: `${Math.max(4, (depth / max) * 100)}%` }} />
            </div>
            <strong className="text-right text-sm text-white">{formatNumber(depth)}</strong>
          </div>
        ))}
      </div>
    </section>
  );
}

function CoreLoopPanel({ frame }: { frame: Frame }) {
  const tickMs = millisFromNanos(frame.tickDurationNanos);
  const redisMs = millisFromNanos(frame.redisLatencyNanos);
  const stages = [
    { label: "Drain ring", value: frame.drainedTickets, tone: "bg-emerald-400" },
    { label: "Search Redis", value: frame.candidateQueries, tone: "bg-sky-400" },
    { label: "Empty lanes", value: frame.emptyQueries, tone: "bg-amber-300" },
    { label: "Assign match", value: frame.matchesLastTick, tone: "bg-pink-400" },
  ];

  return (
    <section className="arena-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Core Loop</span>
          <h2>Match Engine Pulse</h2>
        </div>
        <Activity className="h-7 w-7 text-sky-200" />
      </div>
      <div className="mt-5 grid gap-3 sm:grid-cols-4">
        {stages.map((stage) => (
          <div className="loop-stage" key={stage.label}>
            <span className={`stage-dot ${stage.tone}`} />
            <strong>{formatNumber(stage.value)}</strong>
            <p>{stage.label}</p>
          </div>
        ))}
      </div>
      <div className="loop-track mt-5">
        {stages.map((stage, index) => (
          <div className="loop-node" key={stage.label}>
            <span className={stage.tone} />
            {index < stages.length - 1 ? <i /> : null}
          </div>
        ))}
      </div>
      <div className="mt-5 grid gap-3 sm:grid-cols-4">
        <MiniMetric label="Ticks observed" value={formatNumber(frame.coreTicks)} />
        <MiniMetric label="Tick time" value={`${tickMs.toFixed(2)} ms`} />
        <MiniMetric label="Redis latency" value={`${redisMs.toFixed(2)} ms`} />
        <MiniMetric label="Overruns / skips" value={`${formatNumber(frame.overruns)} / ${formatNumber(frame.skippedTicks)}`} />
      </div>
      <div className="mt-3 grid gap-3 sm:grid-cols-4">
        <MiniMetric label="Total drained" value={formatNumber(frame.totalDrained)} />
        <MiniMetric label="Total searches" value={formatNumber(frame.totalCandidates)} />
        <MiniMetric label="Total matches" value={formatNumber(frame.totalMatches)} />
        <MiniMetric label="Empty searches" value={formatNumber(frame.totalEmptyQueries)} />
      </div>
    </section>
  );
}

function HealthVerdict({ frame }: { frame: Frame }) {
  const tickMs = millisFromNanos(frame.tickDurationNanos);
  const redisMs = millisFromNanos(frame.redisLatencyNanos);
  const budgetUsed = Math.min(100, (tickMs / 200) * 100);
  const matchYield = frame.totalCandidates > 0 ? frame.totalMatches / frame.totalCandidates : 0;
  const verdict =
    frame.overruns > 0 || frame.skippedTicks > 0 ? "Needs attention" : tickMs < 10 && redisMs < 5 ? "Excellent" : "Healthy";
  const tone = verdict === "Excellent" ? "bg-emerald-500" : verdict === "Healthy" ? "bg-sky-500" : "bg-amber-500";

  return (
    <section className="arena-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Service Health</span>
          <h2>{verdict}</h2>
        </div>
        <Gauge className="h-7 w-7 text-emerald-200" />
      </div>
      <div className="mt-5 grid gap-3 sm:grid-cols-4">
        <MiniMetric label="Tick budget used" value={`${budgetUsed.toFixed(3)}%`} />
        <MiniMetric label="Match yield" value={percent(matchYield)} />
        <MiniMetric label="Redis latency" value={`${redisMs.toFixed(2)} ms`} />
        <MiniMetric label="Skipped ticks" value={formatNumber(frame.skippedTicks)} />
      </div>
      <div className="mt-4 rounded-xl bg-white/10 p-3 text-sm font-bold leading-snug text-white/75">
        <span className={`status-pill ${tone} mr-2`}>{verdict}</span>
        This single node is healthy when tick time stays far below 200ms, Redis latency stays low, and overruns/skips stay at zero.
      </div>
    </section>
  );
}

function ArchitecturePanel({ frame }: { frame: Frame }) {
  const nodes = [
    { name: "Player joins", value: frame.totalDrained, detail: "accepted into intake", tone: "bg-amber-300" },
    { name: "Ring buffer", value: frame.drainedTickets, detail: "drained this tick", tone: "bg-emerald-400" },
    { name: "MatchCore", value: frame.coreTicks, detail: "200ms loop pulses", tone: "bg-sky-400" },
    { name: "Redis search", value: frame.totalCandidates, detail: "candidate checks", tone: "bg-cyan-300" },
    { name: "EOMM score", value: frame.eommAccuracy * 100, detail: "match yield", tone: "bg-purple-300", suffix: "%" },
    { name: "Match assigned", value: frame.totalMatches, detail: "Lua commits", tone: "bg-pink-400" },
  ];
  return (
    <section className="arena-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Architecture</span>
          <h2>Live Player Path</h2>
        </div>
        <Bolt className="h-7 w-7 text-amber-200" />
      </div>
      <div className="arch-flow mt-5">
        {nodes.map((node, index) => (
          <React.Fragment key={node.name}>
            <article className="arch-node">
              <span className={`arch-dot ${node.tone}`} />
              <h3>{node.name}</h3>
              <strong>
                {formatNumber(node.value)}
                {node.suffix ?? ""}
              </strong>
              <p>{node.detail}</p>
            </article>
            {index < nodes.length - 1 ? <div className="arch-link" /> : null}
          </React.Fragment>
        ))}
      </div>
    </section>
  );
}

function QueueLauncher({ onAck }: { onAck: (ack: QueueAck) => void }) {
  const [count, setCount] = React.useState(24);
  const [trophies, setTrophies] = React.useState(3200);
  const [churn, setChurn] = React.useState(0.18);
  const [spend, setSpend] = React.useState(0.22);
  const [pool, setPool] = React.useState(0);
  const [sending, setSending] = React.useState(false);

  async function launch() {
    setSending(true);
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${protocol}://${window.location.host}/queue`);
    let sent = 0;
    socket.onopen = () => {
      for (let i = 0; i < count; i++) {
        socket.send(
          JSON.stringify({
            playerId: Date.now() * 1000 + i,
            trophies: trophies + ((i % 7) - 3) * 18,
            deckVector: deckVector(i),
            churnRisk: churn,
            monetizationP: spend,
            poolTag: pool,
            consecLosses: pool === 1 ? 3 : i % 3,
            consecWins: i % 4,
          }),
        );
        sent += 1;
      }
      window.setTimeout(() => socket.close(), 350);
    };
    socket.onmessage = (event) => onAck(JSON.parse(event.data) as QueueAck);
    socket.onclose = () => setSending(false);
    socket.onerror = () => setSending(false);
    if (sent === 0) {
      window.setTimeout(() => setSending(false), 2000);
    }
  }

  return (
    <section className="control-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Queue Injector</span>
          <h2>Deploy Squad</h2>
        </div>
        <Zap className="h-7 w-7 text-emerald-200" />
      </div>
      <div className="mt-5 space-y-4">
        <Slider label="Tickets" value={count} min={1} max={200} onChange={setCount} />
        <Slider label="Trophies" value={trophies} min={0} max={14_000} step={100} onChange={setTrophies} />
        <Slider label="Churn" value={Math.round(churn * 100)} min={0} max={100} suffix="%" onChange={(value) => setChurn(value / 100)} />
        <Slider label="Spend" value={Math.round(spend * 100)} min={0} max={100} suffix="%" onChange={(value) => setSpend(value / 100)} />
        <label className="control-row">
          <span>Pool</span>
          <select className="number-input" value={pool} onChange={(event) => setPool(Number(event.target.value))}>
            {poolNames.map((name, index) => (
              <option key={name} value={index}>
                {index} - {name}
              </option>
            ))}
          </select>
        </label>
      </div>
      <button className="secondary-button mt-5" type="button" onClick={launch} disabled={sending}>
        <Bolt className="h-5 w-5" />
        {sending ? "Deploying" : "Send To Queue"}
      </button>
    </section>
  );
}

function LiveDemoPanel({ frame, onAck }: { frame: Frame; onAck: (ack: QueueAck) => void }) {
  const [running, setRunning] = React.useState(false);
  const [mode, setMode] = React.useState<"burst" | "refill">("burst");
  const [players, setPlayers] = React.useState(500);
  const [joinRate, setJoinRate] = React.useState(100);
  const [trophySpread, setTrophySpread] = React.useState(80);
  const [sent, setSent] = React.useState(0);
  const [acked, setAcked] = React.useState(0);
  const [error, setError] = React.useState("");
  const startTotals = React.useRef({ drained: 0, searches: 0, matches: 0 });
  const stopRef = React.useRef(false);
  const nextPlayerRef = React.useRef(0);
  const lastMatchRef = React.useRef(0);

  React.useEffect(() => {
    return () => {
      stopRef.current = true;
      setRunning(false);
    };
  }, []);

  React.useEffect(() => {
    if (!running || mode !== "refill") {
      lastMatchRef.current = frame.totalMatches;
      return;
    }
    const newMatches = frame.totalMatches - lastMatchRef.current;
    if (newMatches > 0) {
      lastMatchRef.current = frame.totalMatches;
      void sendPlayers(newMatches * 2, Math.max(8, joinRate));
    }
  }, [frame.totalMatches, joinRate, mode, running]);

  async function runLiveDemo() {
    setRunning(true);
    stopRef.current = false;
    setError("");
    setSent(0);
    setAcked(0);
    startTotals.current = {
      drained: frame.totalDrained,
      searches: frame.totalCandidates,
      matches: frame.totalMatches,
    };
    lastMatchRef.current = frame.totalMatches;
    nextPlayerRef.current = Date.now() * 1000;

    try {
      await sendPlayers(players, joinRate);
      if (mode === "burst") {
        setRunning(false);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "queue stream failed");
      setRunning(false);
    }
  }

  function stopLiveDemo() {
    stopRef.current = true;
    setRunning(false);
  }

  function sendPlayers(count: number, ratePerSecond: number) {
    return new Promise<void>((resolve, reject) => {
      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const socket = new WebSocket(`${protocol}://${window.location.host}/queue`);
      const perFlush = Math.max(1, Math.ceil(ratePerSecond / 4));
      let remaining = count;
      let closeTimer = 0;
      let interval = 0;

      socket.onopen = () => {
        const flush = () => {
          if (stopRef.current || remaining <= 0) {
            window.clearInterval(interval);
            closeTimer = window.setTimeout(() => socket.close(), 350);
            return;
          }
          const batchSize = Math.min(perFlush, remaining);
          for (let i = 0; i < batchSize; i++) {
            const index = nextPlayerRef.current++;
            const trophies = 3200 + ((index % Math.max(1, trophySpread)) - Math.floor(trophySpread / 2));
            const playerId = index;
            socket.send(
              JSON.stringify({
                playerId,
                trophies,
                deckVector: deckVector(index),
                churnRisk: 0.12 + (index % 6) * 0.04,
                monetizationP: 0.1 + (index % 5) * 0.05,
                poolTag: 0,
                consecLosses: index % 7 === 0 ? 2 : 0,
                consecWins: index % 4,
              }),
            );
          }
          remaining -= batchSize;
          setSent((value) => value + batchSize);
        };
        flush();
        interval = window.setInterval(flush, 250);
      };
      socket.onmessage = (event) => {
        setAcked((value) => value + 1);
        onAck(JSON.parse(event.data) as QueueAck);
      };
      socket.onerror = () => {
        window.clearInterval(interval);
        window.clearTimeout(closeTimer);
        reject(new Error("queue websocket failed"));
      };
      socket.onclose = () => {
        window.clearInterval(interval);
        window.clearTimeout(closeTimer);
        resolve();
      };
    });
  }

  function runSmallDemo() {
    setPlayers(72);
    setJoinRate(120);
    setMode("burst");
  }

  const drainedDelta = Math.max(0, frame.totalDrained - startTotals.current.drained);
  const searchDelta = Math.max(0, frame.totalCandidates - startTotals.current.searches);
  const matchDelta = Math.max(0, frame.totalMatches - startTotals.current.matches);

  return (
    <section className="control-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Live Matchmaking</span>
          <h2>Player Flow Simulator</h2>
        </div>
        <Swords className="h-7 w-7 text-pink-200" />
      </div>
      <div className="mode-toggle mt-5">
        <button className={mode === "burst" ? "active" : ""} type="button" onClick={() => setMode("burst")}>
          Burst load
        </button>
        <button className={mode === "refill" ? "active" : ""} type="button" onClick={() => setMode("refill")}>
          Live refill
        </button>
      </div>
      <div className="mt-5 space-y-4">
        <Slider label={mode === "burst" ? "Players to join" : "Initial players online"} value={players} min={20} max={50_000} step={20} onChange={setPlayers} />
        <Slider label="Join rate / sec" value={joinRate} min={20} max={2_000} step={20} onChange={setJoinRate} />
        <Slider label="Trophy spread" value={trophySpread} min={10} max={1_000} step={10} onChange={setTrophySpread} />
      </div>
      <div className="mt-5 grid gap-3 sm:grid-cols-2">
        <button className="primary-button" type="button" onClick={runLiveDemo} disabled={running}>
          <Play className="h-5 w-5" />
          {running ? "Running" : mode === "burst" ? "Run Burst" : "Start Refill"}
        </button>
        <button className="secondary-button" type="button" onClick={running ? stopLiveDemo : runSmallDemo}>
          <Bolt className="h-5 w-5" />
          {running ? "Stop" : "Use 72-player demo"}
        </button>
      </div>
      <p className="mt-4 rounded-xl bg-white/10 p-3 text-sm font-bold leading-snug text-white/75">
        Burst load sends a fixed number of players into the real queue. Live refill starts with an online population, then adds two new
        players whenever a match is assigned.
      </p>
      <div className="mt-5 grid gap-3 sm:grid-cols-2">
        <MiniMetric label="Players sent / accepted" value={`${formatNumber(sent)} / ${formatNumber(acked)}`} />
        <MiniMetric label="Drained now" value={formatNumber(drainedDelta)} />
        <MiniMetric label="Searches now" value={formatNumber(searchDelta)} />
        <MiniMetric label="Matches now" value={formatNumber(matchDelta)} />
      </div>
      <div className="demo-flow mt-5">
        <DemoStage label="Player joins" value={sent} tone="bg-amber-300" />
        <DemoStage label="Accepted" value={acked} tone="bg-emerald-400" />
        <DemoStage label="Drained" value={drainedDelta} tone="bg-sky-400" />
        <DemoStage label="Matched" value={matchDelta} tone="bg-pink-400" />
      </div>
      {error ? <p className="mt-3 rounded-xl bg-rose-500/25 px-4 py-2 text-sm font-bold text-white">{error}</p> : null}
    </section>
  );
}

function DemoStage({ label, value, tone }: { label: string; value: number; tone: string }) {
  const lit = Math.min(8, Math.ceil(value / 4));
  return (
    <div className="demo-stage">
      <div className="demo-stage-head">
        <span>{label}</span>
        <strong>{formatNumber(value)}</strong>
      </div>
      <div className="demo-tokens">
        {Array.from({ length: 8 }, (_, index) => (
          <i className={index < lit ? tone : ""} key={index} />
        ))}
      </div>
    </div>
  );
}

function PoolGuide() {
  return (
    <section className="control-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Pool Values</span>
          <h2>Queue Strategy</h2>
        </div>
        <Shield className="h-7 w-7 text-amber-200" />
      </div>
      <div className="mt-5 grid gap-3">
        {poolGuides.map((pool) => (
          <article className="pool-card" key={pool.tag}>
            <div className="flex items-center gap-3">
              <span className="pool-tag">{pool.tag}</span>
              <div className="min-w-0">
                <h3>{pool.name}</h3>
                <p className="pool-short">{pool.short}</p>
              </div>
            </div>
            <p className="pool-detail">{pool.detail}</p>
          </article>
        ))}
      </div>
      <div className="mt-4 rounded-xl bg-white/10 p-3 text-sm font-bold leading-snug text-white/75">
        Pools matter because they keep different player states measurable. If every player join goes through one generic queue, retention, tilt,
        and monetization behavior blur together and the telemetry stops explaining why matches feel good or bad.
      </div>
    </section>
  );
}

function FlowGuide() {
  const steps = [
    ["Frontend", "Sends player joins over /queue and receives telemetry over /telemetry."],
    ["Ring Buffer", "Accepts player joins fast, then matchcore drains them on the 200ms tick."],
    ["Redis Queues", "Stores candidates by trophy segment or special pool so the core can search bounded ranges."],
    ["EOMM Scorer", "Scores candidates using trophy gap, deck vector distance, retention, and monetization terms."],
    ["Telemetry", "Publishes loop counters, queue depth, memory, Redis latency, and EOMM fit back to this deck."],
  ];
  return (
    <section className="arena-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">How It Works</span>
          <h2>Signal Path</h2>
        </div>
        <Bolt className="h-7 w-7 text-amber-200" />
      </div>
      <div className="flow-list mt-5">
        {steps.map(([name, detail], index) => (
          <article className="flow-step" key={name}>
            <span>{index + 1}</span>
            <div>
              <h3>{name}</h3>
              <p>{detail}</p>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function MetricGuide() {
  const items = [
    ["Core ticks", "How many 200ms loop pulses have run since the service started."],
    ["Drain ring", "Tickets moved from the fast in-memory intake buffer into Redis ownership this tick."],
    ["Search Redis", "Candidate range searches issued for drained player joins."],
    ["Assign match", "Successful Redis Lua assignments during the latest tick."],
    ["Empty searches", "Searches that found no legal candidate. High values mean queue shape or tolerance is not lining up."],
    ["EOMM fit", "Total matches divided by total candidate searches. It rises when searches produce assignments."],
    ["Redis latency", "Latest Redis command latency observed by the match loop."],
    ["Queue depth", "Current visible intake depth per ring shard before matchcore drains it."],
  ];
  return (
    <section className="arena-panel">
      <div className="panel-heading">
        <div>
          <span className="kicker">Metric Glossary</span>
          <h2>What To Watch</h2>
        </div>
        <Gauge className="h-7 w-7 text-sky-200" />
      </div>
      <div className="metric-list mt-5">
        {items.map(([name, detail]) => (
          <article className="metric-card" key={name}>
            <h3>{name}</h3>
            <p>{detail}</p>
          </article>
        ))}
      </div>
    </section>
  );
}

function deckVector(index: number) {
  const vector: number[] = Array.from({ length: 8 }, (_, dim) => (dim === index % 8 ? 1 : 0.08));
  vector[(index + 3) % 8] = 0.45;
  return vector;
}

function MiniMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="mini-metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function App() {
  const { frame, connected, history } = useTelemetry();
  const [lastAck, setLastAck] = React.useState<QueueAck | null>(null);
  const totalDepth = frame.queueDepths.reduce((sum, value) => sum + value, 0);
  const tickMs = millisFromNanos(frame.tickDurationNanos);
  const updated = frame.ts > 0 ? new Date(frame.ts / 1_000_000).toLocaleTimeString() : "waiting";

  return (
    <main className="min-h-screen overflow-hidden bg-[#17205a] font-sans text-slate-100">
      <div className="arena-bg" />
      <div className="relative mx-auto flex min-h-screen max-w-[1520px] flex-col px-4 py-4 sm:px-6 lg:px-8">
        <header className="hero-band">
          <div className="min-w-0">
            <div className="flex items-center gap-3">
              <span className="crest">
                <Crown className="h-7 w-7" />
              </span>
              <p className="text-sm font-black uppercase tracking-[0.18em] text-amber-200">MatchPoint Arena Ops</p>
            </div>
            <h1>Battle Deck Control</h1>
          </div>
          <div className="connection-badge">
            {connected ? <Wifi className="h-5 w-5 text-emerald-200" /> : <WifiOff className="h-5 w-5 text-rose-200" />}
            <span>{connected ? "Live" : "Offline"}</span>
            <strong>{updated}</strong>
          </div>
        </header>

        <section className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <StatTile label="Core Ticks" value={formatNumber(frame.coreTicks)} detail={`${tickMs.toFixed(2)} ms latest tick`} tone="gold" icon={<Activity className="h-6 w-6" />} />
          <StatTile label="Matches / Tick" value={formatNumber(frame.matchesLastTick)} detail={`${formatNumber(frame.totalMatches)} total assigned`} tone="pink" icon={<Swords className="h-6 w-6" />} />
          <StatTile label="Queue Depth" value={formatNumber(totalDepth)} detail="players across lanes" tone="emerald" icon={<Trophy className="h-6 w-6" />} />
          <StatTile label="EOMM Fit" value={percent(frame.eommAccuracy)} detail={`${formatNumber(frame.totalCandidates)} scored searches`} tone="sky" icon={<Gem className="h-6 w-6" />} />
        </section>

        <section className="mt-4 grid flex-1 gap-4 xl:grid-cols-[1.15fr_0.85fr]">
          <div className="grid gap-4">
            <HealthVerdict frame={frame} />
            <ArchitecturePanel frame={frame} />
            <CoreLoopPanel frame={frame} />
            <QueueArena depths={frame.queueDepths} />
            <div className="grid gap-4 lg:grid-cols-2">
              <Sparkline label="matches" values={history.map((item) => item.matchesLastTick)} color="#ffd65c" />
              <Sparkline label="tick ms" values={history.map((item) => millisFromNanos(item.tickDurationNanos))} color="#ff77b0" />
            </div>
            <Sparkline label="heap mb" values={history.map((item) => item.allocBytesHeap / 1024 / 1024)} color="#4ad7ff" />
            <FlowGuide />
          </div>
          <aside className="grid content-start gap-4">
            <LiveDemoPanel frame={frame} onAck={setLastAck} />
            <QueueLauncher onAck={setLastAck} />
            <PoolGuide />
            <section className="control-panel">
              <div className="panel-heading">
                <div>
                  <span className="kicker">Last Ack</span>
                  <h2>Dispatch Receipt</h2>
                </div>
                <Activity className="h-7 w-7 text-amber-200" />
              </div>
              <div className="mt-5 grid gap-3">
                <MiniMetric label="Status" value={lastAck?.status ?? "idle"} />
                <MiniMetric label="Player" value={lastAck ? formatNumber(lastAck.playerId) : "none"} />
                <MiniMetric label="Shard / Depth" value={lastAck ? `${lastAck.shard} / ${formatNumber(lastAck.depth)}` : "none"} />
              </div>
            </section>
            <MetricGuide />
          </aside>
        </section>
      </div>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<App />);
