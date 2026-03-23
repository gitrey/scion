import type { MessageEvent } from './types';
import type { AgentRing } from './agents';

const PULSE_DURATION = 600; // ms for pulse travel
const FADE_DURATION = 500;  // ms for line fade after pulse
const LABEL_DURATION = 2000; // ms for content label to remain visible

interface ActiveMessage {
  senderName: string;
  recipientName: string;
  color: string;
  msgType: string;
  content: string;
  startTime: number;
}

export class MessageRenderer {
  private activeMessages: ActiveMessage[] = [];
  private agentRing: AgentRing | null = null;

  setAgentRing(ring: AgentRing): void {
    this.agentRing = ring;
  }

  addMessage(event: MessageEvent, agentRing: AgentRing): void {
    const color = agentRing.getAgentColor(event.sender);

    this.activeMessages.push({
      senderName: event.sender,
      recipientName: event.recipient,
      color,
      msgType: event.msgType,
      content: event.content || '',
      startTime: Date.now(),
    });
  }

  reset(): void {
    this.activeMessages = [];
  }

  draw(ctx: CanvasRenderingContext2D): void {
    const now = Date.now();
    const totalDuration = Math.max(PULSE_DURATION + FADE_DURATION, LABEL_DURATION);

    // Remove expired messages
    this.activeMessages = this.activeMessages.filter(
      (m) => now - m.startTime < totalDuration
    );

    for (const msg of this.activeMessages) {
      const elapsed = now - msg.startTime;

      // Resolve positions dynamically each frame
      const s = this.agentRing?.getAgentPosition(msg.senderName);
      const r = this.agentRing?.getAgentPosition(msg.recipientName);
      if (!s || !r) continue;

      if (elapsed < PULSE_DURATION) {
        // Pulse traveling phase
        const t = elapsed / PULSE_DURATION;

        // Draw line (growing)
        ctx.beginPath();
        ctx.moveTo(s.x, s.y);
        const currentX = s.x + (r.x - s.x) * t;
        const currentY = s.y + (r.y - s.y) * t;
        ctx.lineTo(currentX, currentY);
        ctx.strokeStyle = this.getLineColor(msg, 0.6);
        ctx.lineWidth = this.getLineWidth(msg);
        ctx.stroke();

        // Pulse dot
        ctx.beginPath();
        ctx.arc(currentX, currentY, 4, 0, Math.PI * 2);
        ctx.fillStyle = this.getLineColor(msg, 1);
        ctx.shadowBlur = 10;
        ctx.shadowColor = msg.color;
        ctx.fill();
        ctx.shadowBlur = 0;
      } else if (elapsed < PULSE_DURATION + FADE_DURATION) {
        // Fading phase
        const fadeT = (elapsed - PULSE_DURATION) / FADE_DURATION;
        const alpha = 1 - fadeT;

        ctx.beginPath();
        ctx.moveTo(s.x, s.y);
        ctx.lineTo(r.x, r.y);
        ctx.strokeStyle = this.getLineColor(msg, alpha * 0.6);
        ctx.lineWidth = this.getLineWidth(msg);
        ctx.stroke();
      }

      // Content label along the midpoint of the line
      if (msg.content && elapsed < LABEL_DURATION) {
        const labelAlpha = elapsed < PULSE_DURATION
          ? Math.min(1, (elapsed / PULSE_DURATION) * 2)
          : 1 - (elapsed - PULSE_DURATION) / (LABEL_DURATION - PULSE_DURATION);

        const midX = (s.x + r.x) / 2;
        const midY = (s.y + r.y) / 2;

        // Extract a short summary from content
        const label = this.summarizeContent(msg.content);
        if (label) {
          ctx.save();
          ctx.globalAlpha = Math.max(0, labelAlpha) * 0.9;
          ctx.font = '10px sans-serif';
          ctx.textAlign = 'center';
          ctx.textBaseline = 'bottom';

          // Background pill
          const metrics = ctx.measureText(label);
          const pad = 4;
          const pillW = metrics.width + pad * 2;
          const pillH = 14;
          ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
          roundRect(ctx, midX - pillW / 2, midY - pillH - 4, pillW, pillH, 4);
          ctx.fill();

          // Text
          ctx.fillStyle = this.getLineColor(msg, 1);
          ctx.fillText(label, midX, midY - 5);
          ctx.restore();
        }
      }
    }
  }

  private summarizeContent(content: string): string {
    if (!content) return '';
    // Extract key state from content like "poet-blue has reached a state of COMPLETED: ..."
    const stateMatch = content.match(/state of (\w+)/i);
    if (stateMatch) return stateMatch[1];
    // Truncate long content
    if (content.length > 30) return content.substring(0, 27) + '...';
    return content;
  }

  private getLineColor(msg: ActiveMessage, alpha: number): string {
    if (msg.msgType === 'input-needed') {
      return `rgba(255, 193, 7, ${alpha})`;
    }
    // Parse hex color to rgba
    const hex = msg.color;
    const rr = parseInt(hex.slice(1, 3), 16);
    const g = parseInt(hex.slice(3, 5), 16);
    const b = parseInt(hex.slice(5, 7), 16);
    return `rgba(${rr}, ${g}, ${b}, ${alpha})`;
  }

  private getLineWidth(msg: ActiveMessage): number {
    switch (msg.msgType) {
      case 'instruction':
        return 2.5;
      case 'state-change':
        return 1.5;
      case 'input-needed':
        return 3;
      default:
        return 2;
    }
  }
}

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number
): void {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.lineTo(x + w - r, y);
  ctx.quadraticCurveTo(x + w, y, x + w, y + r);
  ctx.lineTo(x + w, y + h - r);
  ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
  ctx.lineTo(x + r, y + h);
  ctx.quadraticCurveTo(x, y + h, x, y + h - r);
  ctx.lineTo(x, y + r);
  ctx.quadraticCurveTo(x, y, x + r, y);
  ctx.closePath();
}
