import { PageHeader } from "@/shared/layout/PageHeader";

export default function LibraryListRoute() {
  return (
    <div>
      <PageHeader title="Library" subtitle="Your saved articles" />
      <div className="p-4 lg:p-8">
        <p className="font-body">Item list goes here.</p>
      </div>
    </div>
  );
}
