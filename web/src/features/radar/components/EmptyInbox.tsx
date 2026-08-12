export function EmptyInbox() {
  return (
    <div className="text-center py-16 border border-dashed border-rule">
      <p className="label-sc text-muted-foreground mb-3">Inbox zero</p>
      <p className="font-body italic text-muted-foreground">
        Nothing new since you last looked. Standing watch.
      </p>
    </div>
  );
}
