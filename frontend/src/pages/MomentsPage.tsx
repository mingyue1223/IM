import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { ChevronDown, CircleAlert, Globe2, Heart, LoaderCircle, MessageCircle, MoreHorizontal, PenLine, SendHorizontal, Trash2, Users, UsersRound } from "lucide-react";
import { useState } from "react";
import { ApiError } from "../../api/client";
import { Avatar, Button, IconButton } from "../components/ui";
import { useMoments, type MomentView } from "../features/moments/useMoments";
import { useAuthStore } from "../stores/authStore";

const visibilityOptions = [{ value: 1 as const, label: "所有人可见", icon: Globe2 }, { value: 2 as const, label: "仅好友可见", icon: Users }, { value: 3 as const, label: "仅自己可见", icon: UsersRound }];

export function MomentsPage() {
  const [composerOpen, setComposerOpen] = useState(false);
  const [content, setContent] = useState("");
  const [visibility, setVisibility] = useState<1 | 2 | 3>(2);
  const [visibilityOpen, setVisibilityOpen] = useState(false);
  const [commentingId, setCommentingId] = useState<number | null>(null);
  const [commentDrafts, setCommentDrafts] = useState<Record<number, string>>({});
  const [error, setError] = useState<string | null>(null);
  const reduceMotion = useReducedMotion();
  const user = useAuthStore((state) => state.user);
  const username = user?.username ?? "用户";
  const { posts, publish, toggleLike, addComment, deleteComment, isLoading, isError, hasNextPage, fetchNextPage, isFetchingNextPage, isMutating } = useMoments();

  const run = async (operation: () => Promise<void>) => {
    setError(null);
    try { await operation(); }
    catch (failure) { setError(failure instanceof ApiError ? failure.message : "操作失败，请稍后重试"); }
  };
  const submitPost = () => void run(async () => { await publish(content.trim(), visibility); setContent(""); setComposerOpen(false); });
  const submitComment = (post: MomentView) => { const value = commentDrafts[post.id]?.trim(); if (!value) return; void run(async () => { await addComment(post.id, value); setCommentDrafts((current) => ({ ...current, [post.id]: "" })); setCommentingId(null); }); };
  const selectedVisibility = visibilityOptions.find((item) => item.value === visibility)!;
  const VisibilityIcon = selectedVisibility.icon;

  return <><aside className="module-sidebar moments-sidebar"><header className="module-sidebar__header"><div><span className="eyebrow">Moments</span><h1>朋友圈</h1></div></header><div className="moment-me"><Avatar name={username} online size="lg" src={user?.avatarUrl} /><div><strong>{username}</strong><small>记录生活，也保持连接。</small></div></div><Button leadingIcon={<PenLine size={16} />} onClick={() => setComposerOpen(true)}>发布动态</Button><div className="moment-filter-list"><button className="is-active"><UsersRound size={17} /><span>好友动态</span><small>{posts.length}</small></button><button><Users size={17} /><span>我的动态</span></button><button><Globe2 size={17} /><span>可见性设置</span></button></div><p className="moments-sidebar__note">这里是熟人间的动态流。首期仅支持文字内容。</p></aside>
    <section className="module-main moments-main"><header className="feed-header"><div><span className="eyebrow">Friends feed</span><h2>最近动态</h2></div><p>今天也有一些值得分享的事。</p></header>
      <AnimatePresence>{composerOpen && <motion.div animate={{ opacity: 1, height: "auto", y: 0 }} className="moment-composer" exit={{ opacity: 0, height: 0 }} initial={{ opacity: 0, height: 0, y: reduceMotion ? 0 : -8 }}><div className="moment-composer__top"><Avatar name={username} src={user?.avatarUrl} /><textarea autoFocus maxLength={2000} onChange={(event) => setContent(event.target.value)} placeholder="分享此刻的想法…" rows={3} value={content} /></div><footer><div className="visibility-picker"><button onClick={() => setVisibilityOpen((value) => !value)}><VisibilityIcon size={15} />{selectedVisibility.label}<ChevronDown size={14} /></button>{visibilityOpen && <div>{visibilityOptions.map((item) => { const Icon = item.icon; return <button key={item.value} onClick={() => { setVisibility(item.value); setVisibilityOpen(false); }}><Icon size={14} />{item.label}</button>; })}</div>}</div><span><Button onClick={() => { setContent(""); setComposerOpen(false); }} size="sm" variant="ghost">取消</Button><Button disabled={!content.trim() || isMutating} onClick={submitPost} size="sm"><SendHorizontal size={14} />发布</Button></span></footer></motion.div>}</AnimatePresence>
      {error && <div className="feed-error"><CircleAlert size={15} />{error}</div>}
      {isLoading ? <div className="feed-loading"><LoaderCircle className="ui-spinner" size={20} />正在加载动态</div> : isError ? <div className="feed-loading"><CircleAlert size={20} />动态加载失败</div> : <div className="moment-feed">{posts.map((post, index) => <motion.article animate={{ opacity: 1, y: 0 }} className="moment-card" initial={{ opacity: 0, y: reduceMotion ? 0 : 10 }} key={post.id} transition={{ delay: reduceMotion ? 0 : index * .04 }}><header><Avatar name={post.author} /><div><strong>{post.author}</strong><time>{post.time}</time></div><IconButton label="动态操作"><MoreHorizontal size={17} /></IconButton></header><p className="moment-card__content">{post.content}</p><div className="moment-card__actions"><button className={post.liked_by_me ? "is-liked" : ""} disabled={isMutating} onClick={() => void run(() => toggleLike(post))}><Heart fill={post.liked_by_me ? "currentColor" : "none"} size={17} />{post.like_count}</button><button onClick={() => setCommentingId(commentingId === post.id ? null : post.id)}><MessageCircle size={17} />{post.comments.length}</button></div>{post.comments.length > 0 && <div className="moment-interactions">{post.comments.map((comment) => <p className="moment-comment" key={comment.id}><strong>{comment.author}</strong><span>{comment.content}</span>{comment.mine && <button aria-label="删除评论" onClick={() => void run(() => deleteComment(post.id, comment.id))}><Trash2 size={11} /></button>}</p>)}</div>}{commentingId === post.id && <div className="comment-composer"><input autoFocus onChange={(event) => setCommentDrafts((current) => ({ ...current, [post.id]: event.target.value }))} onKeyDown={(event) => { if (event.key === "Enter") submitComment(post); }} placeholder="写下评论，按 Enter 发送" value={commentDrafts[post.id] ?? ""} /><Button disabled={!commentDrafts[post.id]?.trim() || isMutating} onClick={() => submitComment(post)} size="sm">发送</Button></div>}</motion.article>)}{posts.length === 0 && <div className="feed-loading">还没有动态，发布第一条吧。</div>}{hasNextPage && <Button loading={isFetchingNextPage} onClick={() => void fetchNextPage()} variant="secondary">加载更多</Button>}</div>}
    </section></>;
}
