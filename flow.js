// Shared vertical flowchart node, used by posts and replica details.
window.xdigestFlowNode = function (content, index, isLast, extraClass = "") {
  return `<li class="flow-step${isLast ? " is-last" : ""} ${extraClass}" style="--step-i: ${index}">
    <div class="flow-rail" aria-hidden="true"><span class="flow-node"></span>${isLast ? "" : '<span class="flow-line"></span>'}</div>
    <div class="flow-card">${content}</div></li>`;
};
