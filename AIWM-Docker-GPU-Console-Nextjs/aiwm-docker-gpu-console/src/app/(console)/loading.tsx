export default function ConsoleLoading() {
  return <div className="space-y-5"><div className="h-16 animate-pulse rounded-xl bg-slate-200" /><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{Array.from({ length: 4 }).map((_, index) => <div key={index} className="h-36 animate-pulse rounded-xl bg-white" />)}</div><div className="h-80 animate-pulse rounded-xl bg-white" /></div>;
}
