const shapes = {
  shield: <><path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z" /><path d="m8 12 3 3 5-6" /></>,
  profile: <><circle cx="12" cy="8" r="4" /><path d="M4 22v-2a8 8 0 0 1 16 0v2" /></>,
  policies: <><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z" /><path d="M14 3v6h6M8 13h8M8 17h5" /></>,
  quotes: <><rect x="4" y="3" width="16" height="18" rx="2" /><path d="M8 7h8M8 12h4m-4 4h8" /></>,
  plus: <path d="M12 5v14M5 12h14" />,
  claims: <><path d="M3 7h6l2 2h10v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2zM3 7V5a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v2" /></>,
  appraisal: <><circle cx="10" cy="10" r="6" /><path d="m14.5 14.5 5 5M7.5 10h5M10 7.5v5" /></>,
  tasks: <><rect x="4" y="5" width="16" height="16" rx="2" /><rect x="8" y="3" width="8" height="4" rx="1" /><path d="m8 14 3 3 5-6" /></>,
  people: <><circle cx="9" cy="7" r="3" /><path d="M3 21v-3a6 6 0 0 1 12 0v3m1-17a3 3 0 0 1 0 6m3 11v-3a6 6 0 0 0-2-4" /></>,
  wallet: <><path d="M20 8H5a2 2 0 0 1 0-4h13v4M3 6v13a2 2 0 0 0 2 2h15V8M16 12h5v5h-5a2.5 2.5 0 0 1 0-5Z" /></>,
  chart: <path d="M4 3v17h17M8 15v-4m5 4V6m5 9V9" />,
  system: <><rect x="3" y="4" width="18" height="12" rx="2" /><path d="M8 21h8m-4-5v5M7 10h2l2-3 2 6 2-3h2" /></>,
  menu: <path d="M4 6h16M4 12h16M4 18h16" />,
  close: <path d="m6 6 12 12M6 18 18 6" />,
  chevron: <path d="m9 5 7 7-7 7" />,
  logout: <><path d="M9 4H4v16h5m5-13 5 5-5 5M8 12h11" /></>,
  lock: <><rect x="5" y="10" width="14" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 4v3" /></>,
  alert: <><path d="m12 3 10 18H2zM12 9v5m0 3v1" /></>,
  empty: <><path d="m3 8 9-5 9 5v12H3zM3 8l9 5 9-5m-9 5v7" /></>,
}

export type UIIconName = keyof typeof shapes

export function UIIcon({ name }: { name: UIIconName }) {
  return <svg className="ui-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6"
    strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">{shapes[name]}</svg>
}
