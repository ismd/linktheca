import { useState } from "react";
import { gradientClassFor, previewImageUrl } from "../image";

type Props = {
  id: number;
  image: string | null;
};

// The gradient sits underneath, so a missing or broken preview degrades to the
// same backdrop the card uses rather than to an empty box.
export function ReaderHero({ id, image }: Props) {
  const [failed, setFailed] = useState(false);
  const showImage = Boolean(image) && !failed;

  return (
    <div
      className={`${gradientClassFor(id)} relative overflow-hidden w-full h-[280px] md:h-[360px] mb-10`}
    >
      {showImage && (
        <img
          src={previewImageUrl(image!)}
          alt=""
          decoding="async"
          onError={() => setFailed(true)}
          className="absolute inset-0 h-full w-full object-cover"
        />
      )}
    </div>
  );
}