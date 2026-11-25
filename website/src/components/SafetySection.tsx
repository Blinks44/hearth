import { Check, RotateCcw, ShieldCheck } from "lucide-react";
import { useState } from "react";
import * as m from "@/integrations/i18n/paraglide/messages";

export function SafetySection() {
  const [applied, setApplied] = useState(false);

  const rules = [
    { title: m.safetyRule1Title(), desc: m.safetyRule1Desc() },
    { title: m.safetyRule2Title(), desc: m.safetyRule2Desc() },
    { title: m.safetyRule3Title(), desc: m.safetyRule3Desc() },
    { title: m.safetyRule4Title(), desc: m.safetyRule4Desc() },
  ];

  return (
    <section id="safety" className="safety-section" data-reveal>
      <div className="container">
        <div className="safety-card">
          <div className="safety-copy">
            <span className="eyebrow-label">{m.safetyEyebrow()}</span>
            <h2>{m.safetyTitle()}</h2>
            <p>{m.safetySubtitle()}</p>

            <ul className="safety-rules">
              {rules.map((rule, idx) => (
                <li key={idx}>
                  <span className="check-dot" aria-hidden="true">
                    <Check size={12} strokeWidth={2.8} />
                  </span>
                  <div>
                    <strong>{rule.title}</strong>
                    <span className="safety-rule-desc">{rule.desc}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>

          <div className="diff-dialog">
            <div className="diff-dialog-head">
              <div>
                <h3>{m.safetyDialogTitle()}</h3>
                <p>{m.safetyDialogSub()}</p>
              </div>
              <span
                className="pill pill-success"
                style={{ textTransform: "none", letterSpacing: 0 }}
              >
                <ShieldCheck size={13} aria-hidden="true" />
                {applied ? m.safetyVerifiedPill() : m.safetyBackupPill()}
              </span>
            </div>

            <div className="diff-box">
              <div className="diff-box-bar">
                <strong>~/.zshrc</strong>
                <span>Backup: ~/.zshrc.hearth-20261123.bak</span>
              </div>
              <div className="diff-lines">
                <div className="diff-line diff-ctx">
                  <span className="diff-ln">41</span>
                  <span> export EDITOR=&quot;code --wait&quot;</span>
                </div>
                {!applied ? (
                  <>
                    <div className="diff-line diff-del">
                      <span className="diff-ln">42</span>
                      <span>- export PATH=&quot;/opt/homebrew/opt/node@18/bin:$PATH&quot;</span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">42</span>
                      <span>+ # export PATH=&quot;/opt/homebrew/opt/node@18/bin:$PATH&quot;</span>
                    </div>
                    <div className="diff-line diff-ctx">
                      <span className="diff-ln">43</span>
                      <span> </span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">44</span>
                      <span>+ # &gt;&gt;&gt; hearth &gt;&gt;&gt;</span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">45</span>
                      <span>+ eval &quot;$(mise activate zsh)&quot;</span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">46</span>
                      <span>+ # &lt;&lt;&lt; hearth &lt;&lt;&lt;</span>
                    </div>
                  </>
                ) : (
                  <>
                    <div className="diff-line diff-ctx">
                      <span className="diff-ln">42</span>
                      <span> # export PATH=&quot;/opt/homebrew/opt/node@18/bin:$PATH&quot;</span>
                    </div>
                    <div className="diff-line diff-ctx">
                      <span className="diff-ln">43</span>
                      <span> </span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">44</span>
                      <span> # &gt;&gt;&gt; hearth &gt;&gt;&gt;</span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">45</span>
                      <span> eval &quot;$(mise activate zsh)&quot;</span>
                    </div>
                    <div className="diff-line diff-add">
                      <span className="diff-ln">46</span>
                      <span> # &lt;&lt;&lt; hearth &lt;&lt;&lt;</span>
                    </div>
                  </>
                )}
              </div>
            </div>

            <div className="diff-next" data-verified={applied}>
              <strong>{applied ? m.safetyVerifiedPill() : m.safetyNextHeading()}</strong>
              <span>{applied ? m.safetyAppliedBody() : m.safetyNextBody()}</span>
            </div>

            <div className="diff-actions">
              {applied ? (
                <button
                  type="button"
                  className="btn-hearth-outline"
                  onClick={() => setApplied(false)}
                >
                  <RotateCcw size={14} aria-hidden="true" />
                  <span>{m.safetyResetBtn()}</span>
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    className="btn-hearth-outline"
                    onClick={() => setApplied(false)}
                  >
                    {m.safetyCancelBtn()}
                  </button>
                  <button
                    type="button"
                    className="btn-hearth-dark"
                    onClick={() => setApplied(true)}
                  >
                    {m.safetyApplyBtn()}
                  </button>
                </>
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
