(() => {
  const form = document.querySelector(".apply-form");
  if (!form) return;

  const checkbox = form.querySelector('input[name="understands_stream_required"]');
  const submit = form.querySelector('button[type="submit"]');
  if (!checkbox || !submit) return;

  const syncSubmitState = () => {
    submit.disabled = !checkbox.checked;
  };

  checkbox.addEventListener("change", syncSubmitState);
  syncSubmitState();
})();
