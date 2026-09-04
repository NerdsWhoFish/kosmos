import { ReactNode } from "react";
import { ArrowLeft } from "lucide-react";

export function WorkflowPage({
  eyebrow,
  title,
  detail,
  backLabel,
  onBack,
  children,
  tone = "default",
}: {
  eyebrow: string;
  title: string;
  detail?: string;
  backLabel: string;
  onBack: () => void;
  children: ReactNode;
  tone?: "default" | "danger";
}) {
  return (
    <div className={`page workflow-page ${tone === "danger" ? "danger" : ""}`}>
      <button className="back-button" onClick={onBack}>
        <ArrowLeft size={16} /> {backLabel}
      </button>
      <header className="page-heading workflow-heading">
        <div>
          <p className="eyebrow">{eyebrow}</p>
          <h1>{title}</h1>
          {detail && <p className="subhead">{detail}</p>}
        </div>
      </header>
      <section className="panel workflow-card">{children}</section>
    </div>
  );
}
