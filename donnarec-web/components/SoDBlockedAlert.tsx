import { AlertTriangle } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";

/**
 * SoDBlockedAlert — amber alert shown when a Checker views their own pending record.
 *
 * UI-SPEC §Screen 3 "SoD Blocked State":
 *   Alert component: amber-50 background, AlertTriangle icon, amber-600 text.
 *   Locked message per UI-SPEC (Thai only).
 *
 * When this alert is shown, the approve / return / reject controls are
 * completely ABSENT from the DOM (not disabled) — T-03-31.
 * The server is the authority; this is UX-layer defense only.
 */
export function SoDBlockedAlert() {
  return (
    <Alert className="border-amber-200 bg-amber-50">
      <AlertTriangle className="h-4 w-4 text-amber-600" />
      <AlertDescription className="text-[14px] text-amber-700">
        คุณเป็นผู้สร้างรายการนี้ — ผู้อนุมัติต้องเป็นบุคคลอื่นตามหลักการแยกหน้าที่
        (Segregation of Duties) กรุณาให้ Checker ท่านอื่นดำเนินการ
      </AlertDescription>
    </Alert>
  );
}
