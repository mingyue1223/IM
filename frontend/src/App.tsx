import {
  Bell,
  Check,
  ChevronRight,
  Inbox,
  LayoutGrid,
  MessageCircle,
  MoreHorizontal,
  Search,
  Settings2,
  Sparkles,
  Users,
} from "lucide-react";
import { useCallback, useState } from "react";
import {
  Avatar,
  Badge,
  Button,
  ConfirmDialog,
  Drawer,
  EmptyState,
  IconButton,
  Skeleton,
  Surface,
  Switch,
  TextField,
  ToastViewport,
  type ToastItem,
} from "./components/ui";

const palette = [
  { name: "Canvas", value: "#f4f5f7", className: "token-swatch--canvas" },
  { name: "Surface", value: "#ffffff", className: "token-swatch--surface" },
  { name: "Ink", value: "#1c1d21", className: "token-swatch--ink" },
  { name: "Accent", value: "#5c72e8", className: "token-swatch--accent" },
  { name: "Success", value: "#35a66f", className: "token-swatch--success" },
];

const conversations = [
  { name: "林澄", message: "项目的方向很清晰", time: "10:24", unread: 2, online: true },
  { name: "产品讨论组", message: "周一一起看第一版", time: "09:48", unread: 0 },
  { name: "周屿", message: "好的，晚点见", time: "昨天", unread: 0, online: false },
];

function App() {
  const [notifications, setNotifications] = useState(true);
  const [preview, setPreview] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const showToast = useCallback((message: string, tone: ToastItem["tone"] = "success") => {
    const id = crypto.randomUUID();
    setToasts((current) => [...current, { id, message, tone }]);
    window.setTimeout(() => setToasts((current) => current.filter((toast) => toast.id !== id)), 2600);
  }, []);

  return (
    <main className="foundation-page">
      <aside className="foundation-sidebar">
        <div className="brand-mark"><Sparkles size={18} /><span>GoIM</span></div>
        <nav aria-label="设计系统导航">
          <a className="is-active" href="#overview"><LayoutGrid size={17} />概览</a>
          <a href="#controls"><Settings2 size={17} />控件</a>
          <a href="#states"><Inbox size={17} />状态</a>
        </nav>
        <p>Foundation · Phase 01</p>
      </aside>

      <section className="foundation-content">
        <header className="foundation-hero" id="overview">
          <div>
            <Badge>设计基础已就绪</Badge>
            <h1>轻盈、清晰，留一点呼吸。</h1>
            <p>GoIM 的浅色设计令牌与通用组件预览。这里用于第一阶段的视觉基线确认，不承载正式业务。</p>
          </div>
          <Button leadingIcon={<Check size={16} />} onClick={() => showToast("设计令牌已应用")}>查看反馈</Button>
        </header>

        <section className="foundation-section">
          <div className="section-heading"><div><span>01</span><h2>颜色与材质</h2></div><p>低饱和强调色配合雾灰画布，使用光影而非重边框建立层级。</p></div>
          <div className="token-grid">
            {palette.map((color) => (
              <Surface className="token-card" key={color.name}>
                <span className={`token-swatch ${color.className}`} />
                <div><strong>{color.name}</strong><code>{color.value}</code></div>
              </Surface>
            ))}
          </div>
        </section>

        <section className="foundation-section" id="controls">
          <div className="section-heading"><div><span>02</span><h2>核心控件</h2></div><p>统一焦点、禁用、错误与操作反馈，供后续业务页面直接组合。</p></div>
          <div className="component-grid">
            <Surface className="component-card">
              <header><h3>按钮</h3><p>主次层级保持克制。</p></header>
              <div className="component-row">
                <Button>主要操作</Button>
                <Button variant="secondary">次要操作</Button>
                <Button variant="ghost">轻操作</Button>
                <IconButton label="更多操作"><MoreHorizontal size={18} /></IconButton>
              </div>
              <div className="component-row">
                <Button size="sm">小尺寸</Button>
                <Button loading>处理中</Button>
                <Button disabled>不可用</Button>
                <Button variant="danger" onClick={() => setDialogOpen(true)}>危险操作</Button>
              </div>
            </Surface>

            <Surface className="component-card">
              <header><h3>输入</h3><p>标签、辅助文本与错误信息位置稳定。</p></header>
              <TextField label="搜索" leadingIcon={<Search size={16} />} placeholder="搜索联系人或会话" />
              <TextField error="请输入有效内容" label="消息" placeholder="在这里输入" />
            </Surface>

            <Surface className="component-card">
              <header><h3>偏好设置</h3><p>轻量切换，不打断当前任务。</p></header>
              <Switch checked={notifications} label="消息通知" description="有新消息时显示桌面通知" onCheckedChange={setNotifications} />
              <Switch checked={preview} label="消息预览" description="在通知中显示消息正文" onCheckedChange={setPreview} />
            </Surface>

            <Surface className="component-card">
              <header><h3>头像与状态</h3><p>统一尺寸和在线状态表达。</p></header>
              <div className="avatar-row">
                <Avatar name="林澄" online size="xl" />
                <Avatar name="周屿" online={false} size="lg" />
                <Avatar name="产品组" size="md" />
                <Avatar name="GoIM" size="sm" />
              </div>
              <Button size="sm" variant="secondary" onClick={() => setDrawerOpen(true)}>打开资料抽屉</Button>
            </Surface>
          </div>
        </section>

        <section className="foundation-section" id="states">
          <div className="section-heading"><div><span>03</span><h2>列表与系统状态</h2></div><p>为加载、空内容和常见会话结构建立统一节奏。</p></div>
          <div className="state-grid">
            <Surface className="conversation-preview">
              <header><div><h3>最近会话</h3><p>3 个会话</p></div><IconButton label="会话设置"><MoreHorizontal size={18} /></IconButton></header>
              <div className="conversation-list">
                {conversations.map((item, index) => (
                  <button className={index === 0 ? "is-active" : ""} key={item.name}>
                    <Avatar name={item.name} online={item.online} />
                    <span className="conversation-copy"><strong>{item.name}</strong><small>{item.message}</small></span>
                    <span className="conversation-meta"><time>{item.time}</time>{item.unread > 0 && <b>{item.unread}</b>}</span>
                  </button>
                ))}
              </div>
            </Surface>

            <Surface className="system-state-card">
              <EmptyState
                action={{ label: "添加联系人", onClick: () => showToast("即将进入联系人模块", "info") }}
                description="添加好友后，就可以从这里开始一段对话。"
                icon={<MessageCircle size={25} />}
                title="还没有会话"
              />
              <div className="skeleton-block" aria-label="加载状态预览">
                <Skeleton className="skeleton-avatar" />
                <div><Skeleton /><Skeleton className="is-short" /></div>
              </div>
            </Surface>
          </div>
        </section>
      </section>

      <Drawer description="用于联系人和群聊资料，不挤压主内容区。" onClose={() => setDrawerOpen(false)} open={drawerOpen} title="资料抽屉">
        <div className="drawer-profile">
          <Avatar name="林澄" online size="xl" />
          <div><h3>林澄</h3><p>在线</p></div>
        </div>
        <div className="drawer-menu">
          <button><span><Bell size={17} />消息通知</span><ChevronRight size={17} /></button>
          <button><span><Users size={17} />共同群聊</span><ChevronRight size={17} /></button>
        </div>
      </Drawer>

      <ConfirmDialog
        confirmLabel="确认删除"
        description="此操作无法撤销，请确认是否继续。"
        destructive
        onClose={() => setDialogOpen(false)}
        onConfirm={() => { setDialogOpen(false); showToast("操作已完成", "success"); }}
        open={dialogOpen}
        title="删除这项内容？"
      />
      <ToastViewport items={toasts} />
    </main>
  );
}

export default App;
