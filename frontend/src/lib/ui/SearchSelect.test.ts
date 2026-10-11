import { fireEvent, render, screen } from "@testing-library/svelte";
import { expect, test, vi } from "vitest";
import SearchSelect from "./SearchSelect.svelte";

const options = [
  { value: "gpt-5", label: "gpt-5", detail: "openai" },
  { value: "claude-sonnet", label: "claude-sonnet", detail: "anthropic" },
  { value: "claude-opus", label: "claude-opus", detail: "anthropic" },
];

function setup(value = "") {
  render(SearchSelect, {
    label: "Model",
    id: "model",
    options,
    value,
    emptyLabel: "None",
  });
  return screen.getByRole("combobox", { name: "Model" }) as HTMLInputElement;
}

test("typing filters by label and detail", async () => {
  const input = setup();
  await fireEvent.input(input, { target: { value: "anthropic" } });
  const names = screen
    .getAllByRole("option")
    .map((option) => option.textContent);
  expect(names).toHaveLength(2);
  expect(names.every((name) => name?.includes("claude"))).toBe(true);
  await fireEvent.input(input, { target: { value: "nothing-here" } });
  expect(screen.queryAllByRole("option")).toHaveLength(0);
  expect(screen.getByText("No matches")).toBeTruthy();
});

test("arrow keys and Enter pick an option", async () => {
  const input = setup();
  await fireEvent.input(input, { target: { value: "claude" } });
  await fireEvent.keyDown(input, { key: "ArrowDown" });
  expect(input.getAttribute("aria-activedescendant")).toBe("model-opt-1");
  await fireEvent.keyDown(input, { key: "Enter" });
  expect(input.value).toBe("claude-opus");
  expect(input.getAttribute("aria-expanded")).toBe("false");
});

test("Escape closes and restores the selected label", async () => {
  const input = setup("gpt-5");
  await fireEvent.input(input, { target: { value: "clau" } });
  expect(input.getAttribute("aria-expanded")).toBe("true");
  await fireEvent.keyDown(input, { key: "Escape" });
  expect(input.getAttribute("aria-expanded")).toBe("false");
  expect(input.value).toBe("gpt-5");
});

test("onchange fires only when the value changes", async () => {
  const onchange = vi.fn();
  render(SearchSelect, {
    label: "Project",
    id: "project",
    options,
    value: "gpt-5",
    onchange,
  });
  const input = screen.getByRole("combobox", { name: "Project" });
  await fireEvent.click(input);
  await fireEvent.click(screen.getByRole("option", { name: /gpt-5/ }));
  expect(onchange).not.toHaveBeenCalled();
  await fireEvent.click(input);
  await fireEvent.click(screen.getByRole("option", { name: /claude-opus/ }));
  expect(onchange).toHaveBeenCalledWith("claude-opus");
});

test("a disabled option shows but cannot be picked", async () => {
  const onchange = vi.fn();
  const withLocked = [
    ...options,
    {
      value: "locked",
      label: "locked",
      detail: "Other tribe: Payments",
      disabled: true,
    },
  ];
  render(SearchSelect, {
    label: "Linked",
    id: "linked",
    options: withLocked,
    value: "",
    onchange,
  });
  const input = screen.getByRole("combobox", {
    name: "Linked",
  }) as HTMLInputElement;
  await fireEvent.input(input, { target: { value: "locked" } });
  const locked = screen.getByRole("option", { name: /locked/ });
  expect(locked.getAttribute("aria-disabled")).toBe("true");
  await fireEvent.click(locked);
  await fireEvent.keyDown(input, { key: "Enter" });
  expect(onchange).not.toHaveBeenCalled();
  expect(input.getAttribute("aria-expanded")).toBe("true");
});

test("the empty option clears the value", async () => {
  const input = setup("gpt-5");
  await fireEvent.click(input);
  const none = screen.getByRole("option", { name: "None" });
  expect(
    screen.getByRole("option", { name: /gpt-5/ }).getAttribute("aria-selected"),
  ).toBe("true");
  await fireEvent.click(none);
  expect(input.value).toBe("");
});
