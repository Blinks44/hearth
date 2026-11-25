import * as m from "@/integrations/i18n/paraglide/messages";

export function NativeMetrics() {
  const items = [
    { value: m.metric1Value(), title: m.metric1Title(), desc: m.metric1Desc(), accent: true },
    { value: m.metric2Value(), title: m.metric2Title(), desc: m.metric2Desc(), accent: false },
    { value: m.metric3Value(), title: m.metric3Title(), desc: m.metric3Desc(), accent: false },
    { value: m.metric4Value(), title: m.metric4Title(), desc: m.metric4Desc(), accent: false },
  ];

  return (
    <section className="metrics-section" data-reveal>
      <div className="container">
        <div className="metrics-grid">
          {items.map((item, idx) => (
            <div key={idx} className="metric-card">
              <div className={item.accent ? "metric-value metric-value-accent" : "metric-value"}>
                {item.value}
              </div>
              <div className="metric-title">{item.title}</div>
              <p className="metric-desc">{item.desc}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
