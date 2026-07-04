import React from "react";
import { createRoot } from "react-dom/client";
import { Activity, Cpu, RadioTower, ShieldAlert, Swords, Wifi, WifiOff } from "lucide-react";
import "./style.css";

type Frame = {
  ts: number;
  queueDepths: number[];
  matchesLastTick: number;
  eommAccuracy: number;
  allocBytesHeap: number;
  churnAlerts: number;
};

const emptyFrame: Frame = {
  ts: 0,
  queueDepths: [0, 0, 0, 0, 0],
  matchesLastTick: 0,
  eommAccuracy: 0,
  allocBytesHeap: 0,
  churnAlerts: 0,
};

function useTelemetry() {
  const [frame, setFrame] = React.useState<Frame>(emptyFrame);
  const [connected, setConnected] = React.useState(false);

  React.useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${protocol}://${window.location.host}/telemetry`);
    socket.onopen = () => setConnected(true);
    socket.onclose = () => setConnected(false);
    socket.onerror = () => setConnected(false);
    socket.onmessage = (event) => {
      setFrame(JSON.parse(event.data) as Frame);
    };
    return () => socket.close();
  }, []);

  return { frame, connected };
}

function StatCard({
  label,
  value,
  accent,
  children,
}: {
  label: string;
  value: string;
  accent: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-lg border border-white/10 bg-slate-950/70 p-4 shadow-terminal backdrop-blur">
      <div className="flex items-center justify-between">
        <p className="font-mono text-xs uppercase tracking-[0.24em] text-slate-400">{label}</p>
        <div className={`rounded-md border border-white/10 p-2 ${accent}`}>{children}</div>
      </div>
      <p className="mt-4 font-mono text-3xl font-black text-white">{value}</p>
    </section>
  );
}

function QueueBars({ depths }: { depths: number[] }) {
  const max = Math.max(1, ...depths);
  return (
    <section className="rounded-lg border border-neon/20 bg-black/55 p-5 shadow-terminal">
      <div className="mb-5 flex items-center justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.26em] text-neon">Queue Grid</p>
          <h2 className="mt-1 text-xl font-bold text-white">Redis segment depth</h2>
        </div>
        <RadioTower className="h-5 w-5 text-cyan" />
      </div>
      <div className="space-y-3">
        {depths.map((depth, index) => (
          <div key={index} className="grid grid-cols-[88px_1fr_72px] items-center gap-3">
            <span className="font-mono text-xs text-slate-400">SEG-{index}</span>
            <div className="h-7 overflow-hidden rounded bg-slate-900 ring-1 ring-white/10">
              <div
                className="h-full rounded bg-gradient-to-r from-neon via-cyan to-fuchsia-400 shadow-glow transition-[width] duration-200"
                style={{ width: `${(depth / max) * 100}%` }}
              />
            </div>
            <span className="text-right font-mono text-sm text-white">{depth}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

function App() {
  const { frame, connected } = useTelemetry();
  const heapMB = frame.allocBytesHeap / 1024 / 1024;
  const updated = frame.ts > 0 ? new Date(frame.ts / 1_000_000).toLocaleTimeString() : "waiting";

  return (
    <main className="min-h-screen bg-void text-slate-100">
      <div className="fixed inset-0 bg-[radial-gradient(circle_at_20%_10%,rgba(54,216,255,0.15),transparent_28%),linear-gradient(rgba(57,255,136,0.07)_1px,transparent_1px),linear-gradient(90deg,rgba(57,255,136,0.05)_1px,transparent_1px)] bg-[size:auto,28px_28px,28px_28px]" />
      <div className="relative mx-auto flex min-h-screen max-w-7xl flex-col px-5 py-6">
        <header className="mb-6 flex flex-wrap items-end justify-between gap-4 border-b border-neon/20 pb-5">
          <div>
            <p className="font-mono text-xs uppercase tracking-[0.35em] text-neon">MatchPoint Ops</p>
            <h1 className="mt-2 text-4xl font-black text-white sm:text-5xl">Telemetry Deck</h1>
          </div>
          <div className="flex items-center gap-3 rounded-lg border border-white/10 bg-slate-950/80 px-4 py-3 font-mono text-sm">
            {connected ? <Wifi className="h-4 w-4 text-neon" /> : <WifiOff className="h-4 w-4 text-danger" />}
            <span className={connected ? "text-neon" : "text-danger"}>{connected ? "LINK ONLINE" : "LINK DOWN"}</span>
            <span className="text-slate-500">/</span>
            <span className="text-slate-300">{updated}</span>
          </div>
        </header>

        <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <StatCard label="Matches / tick" value={String(frame.matchesLastTick)} accent="text-neon">
            <Swords className="h-5 w-5" />
          </StatCard>
          <StatCard label="EOMM accuracy" value={frame.eommAccuracy.toFixed(2)} accent="text-cyan">
            <Activity className="h-5 w-5" />
          </StatCard>
          <StatCard label="Heap memory" value={`${heapMB.toFixed(1)} MB`} accent="text-fuchsia-300">
            <Cpu className="h-5 w-5" />
          </StatCard>
          <StatCard label="Churn alerts" value={String(frame.churnAlerts)} accent="text-danger">
            <ShieldAlert className="h-5 w-5" />
          </StatCard>
        </section>

        <section className="mt-5 grid flex-1 gap-5 lg:grid-cols-[1.3fr_0.7fr]">
          <QueueBars depths={frame.queueDepths} />
          <aside className="rounded-lg border border-white/10 bg-slate-950/70 p-5 shadow-terminal">
            <p className="font-mono text-xs uppercase tracking-[0.26em] text-cyan">System Pulse</p>
            <div className="mt-5 space-y-4 font-mono text-sm">
              <div className="flex justify-between border-b border-white/10 pb-3">
                <span className="text-slate-400">ws_endpoint</span>
                <span className="text-neon">/telemetry</span>
              </div>
              <div className="flex justify-between border-b border-white/10 pb-3">
                <span className="text-slate-400">frame_rate</span>
                <span className="text-white">10Hz</span>
              </div>
              <div className="flex justify-between border-b border-white/10 pb-3">
                <span className="text-slate-400">ring_slots</span>
                <span className="text-white">65,536</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-400">render_mode</span>
                <span className="text-cyan">react/tailwind</span>
              </div>
            </div>
          </aside>
        </section>
      </div>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<App />);
