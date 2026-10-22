// Shared vertical flowchart node, used by posts and replica details.
window.xdigestFlowNode = function (content, index, isLast, extraClass = "", statusControl = "") {
  return `<li class="flow-step${isLast ? " is-last" : ""} ${extraClass}" style="--step-i: ${index}">
    <div class="flow-rail">${statusControl || '<span class="flow-node" aria-hidden="true"></span>'}${isLast ? "" : '<span class="flow-line" aria-hidden="true"></span>'}</div>
    <div class="flow-card">${content}</div></li>`;
};
