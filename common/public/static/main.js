let hiddenTextArea = undefined;

function showUploadModal() {
    if (location.href.split('/')[3].startsWith("explorer")) {
        let path = getPathParam();
        document.getElementById('uploadFileDialogTitle').innerText = `上传文件到 "${path}"`;
    }
    showModal('uploadModal');
}

function getPathParam() {
    let url = new URL(location.href);
    let searchParams = new URLSearchParams(url.search);
    let path = "/";
    if (searchParams.has('path')) {
        path = searchParams.get('path');
    }
    if (path === "") path = "/";
    return path;
}

function closeUploadModal() {
    document.getElementById('uploadModal').className = "modal";
}


function showModal(id) {
    document.getElementById(id).className = "modal is-active";
}

function closeModal(id) {
    document.getElementById(id).className = "modal";
}

function onChooseBtnClicked(e) {
    document.getElementById('fileInput').click();
    e.preventDefault();
}

async function deleteFile(id, link) {
    try {
        let data = await requestJSON("/api/file", {
            method: 'delete',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                id: id,
                link: link
            })
        });
        if (!data.success) {
            console.error(data.message);
            showMessage(data.message, true);
        } else {
            document.getElementById("file-" + id).style.display = 'none';
            showToast(`文件删除成功：${link}`)
        }
    } catch (e) {
        showMessage(`删除失败：${e.message}`, true);
    }
}

async function deleteImage() {
    let e = document.getElementById("inputDeleteImage");
    if (e.value === "") return;
    let tmpList = e.value.split("/");
    let filename = tmpList[tmpList.length - 1];

    try {
        let data = await requestJSON("/api/image", {
            method: 'delete',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                filename: filename,
            })
        });
        if (data.success) {
            e.value = "";
            showToast("图片已成功删除");
        } else {
            showToast(data.message, "danger");
        }
    } catch (err) {
        showToast(`删除失败：${err.message}`, "danger");
    }
}


function updateDownloadCounter(id) {
    let e = document.getElementById(id);
    let n = parseInt(e.innerText.replace("次下载", ""));
    e.innerText = `${n + 1} 次下载`;
}

function onFileInputChange() {
    let prompt;
    let files = document.getElementById('fileInput').files;
    if (files.length === 1) {
        prompt = '已选择文件: ' + files[0].name;
    } else {
        prompt = `已选择 ${files.length} 个文件`;
    }
    document.getElementById('uploadFileDialogTitle').innerText = prompt;
}

function byte2mb(n) {
    let sizeMb = 1024 * 1024;
    n /= sizeMb;
    return n.toFixed(2);
}

// firstOversizedFile returns the first file exceeding the configured upload
// size limit, or null when every file is within the limit (or no limit set).
function firstOversizedFile(files) {
    let limitMb = window.maxUploadSizeMB || 0;
    if (limitMb <= 0) {
        return null;
    }
    let limitBytes = limitMb * 1024 * 1024;
    for (let i = 0; i < files.length; i++) {
        if (files[i].size > limitBytes) {
            return files[i];
        }
    }
    return null;
}

function uploadFile() {
    let fileUploadCard = document.getElementById('fileUploadCard');
    let fileUploadTitle = document.getElementById('fileUploadTitle');
    let fileUploadProgress = document.getElementById('fileUploadProgress');
    let fileUploadDetail = document.getElementById('fileUploadDetail');
    fileUploadCard.style.display = 'block';
    let files = document.getElementById('fileInput').files;
    let description = document.getElementById("fileUploadDescription").value;
    if (files.length === 0 && description === "") {
        return;
    }
    let oversized = firstOversizedFile(files);
    if (oversized) {
        fileUploadCard.style.display = 'none';
        showMessage(`文件 ${oversized.name}（${byte2mb(oversized.size)} MB）超过上传大小限制 ${window.maxUploadSizeMB} MB`, true);
        return;
    }
    closeUploadModal();
    let formData = new FormData();
    for (let i = 0; i < files.length; i++) {
        formData.append("file", files[i]);
    }
    formData.append("description", description);

    let path = "";
    if (location.href.split('/')[3].startsWith("explorer")) {
        path = getPathParam();
    }
    formData.append("path", path);

    fileUploadTitle.innerText = `正在上传 ${files.length} 个文件`;

    let fileUploader = new XMLHttpRequest();
    fileUploader.upload.addEventListener("progress", ev => {
        let percent = (ev.loaded / ev.total) * 100;
        fileUploadProgress.value = Math.round(percent);
        fileUploadDetail.innerText = `处理中 ${byte2mb(ev.loaded)} MB / ${byte2mb(ev.total)} MB...`
    }, false);
    fileUploader.addEventListener("load", ev => {
        if (fileUploader.status === 403) {
            location.href = "/login";
            return;
        }
        if (fileUploader.status >= 200 && fileUploader.status < 400) {
            fileUploadTitle.innerText = `已上传 ${files.length} 个文件`;
            location.reload();
            return;
        }
        // 上传失败：把服务器返回的具体原因展示给用户，便于定位问题。
        fileUploadCard.style.display = 'none';
        let detail = (fileUploader.responseText || "").trim();
        let hint = detail !== "" ? detail : `服务器返回状态码 ${fileUploader.status}`;
        showMessage(`文件上传失败：${hint}`, true);
        console.error("upload failed", fileUploader.status, detail);
    }, false);
    fileUploader.addEventListener("error", ev => {
        if (fileUploader.status === 403) {
            location.href = "/login";
            return;
        }
        fileUploadCard.style.display = 'none';
        showMessage(`文件上传失败：网络错误或服务器无响应，请检查网络后重试`, true);
        console.error(ev);
    }, false);
    fileUploader.addEventListener("abort", ev => {
        fileUploadTitle.innerText = `文件上传已终止`;
    }, false);
    fileUploader.open("POST", "/api/file");
    fileUploader.send(formData);
}

function dropHandler(ev) {
    ev.preventDefault();
    document.getElementById('fileInput').files = ev.dataTransfer.files;
    onFileInputChange();
}

function dragOverHandler(ev) {
    document.getElementById('uploadFileDialogTitle').innerText = "释放文件至此对话框";
    ev.preventDefault();
}

function imageDropHandler(ev) {
    ev.preventDefault();
    document.getElementById('fileInput').files = ev.dataTransfer.files;
    uploadImage();
}

function uploadImage() {
    document.getElementById("promptBox").style.display = "block";
    let imageUploadProgress = document.getElementById('imageUploadProgress');
    let imageUploadStatus = document.getElementById('imageUploadStatus');
    imageUploadStatus.innerText = "上传中..."

    let files = document.getElementById('fileInput').files;
    let images = [];
    for (let i = 0; i < files.length; i++) {
        if (files[i]['type'].split('/')[0] === 'image') {
            images.push(files[i]);
        }
    }
    let oversized = firstOversizedFile(images);
    if (oversized) {
        document.getElementById("promptBox").style.display = "none";
        showMessage(`图片 ${oversized.name}（${byte2mb(oversized.size)} MB）超过上传大小限制 ${window.maxUploadSizeMB} MB`, true);
        return;
    }
    let formData = new FormData();
    images.forEach(img => formData.append("image", img));

    let fileUploader = new XMLHttpRequest();
    fileUploader.upload.addEventListener("progress", ev => {
        let percent = (ev.loaded / ev.total) * 100;
        imageUploadProgress.value = Math.round(percent);
    }, false);
    fileUploader.addEventListener("load", ev => {
        // 上传完成。load 事件最后触发，作为状态文案的唯一来源，避免与
        // readystatechange 互相覆盖。
        if (fileUploader.status === 403) {
            location.href = "/login";
            return;
        }
        if (fileUploader.status === 200) {
            imageUploadStatus.innerText = "文件上传成功";
            return;
        }
        // 上传失败：优先展示服务器返回的 message 字段，便于定位问题。
        let hint = `服务器返回状态码 ${fileUploader.status}`;
        let detail = (fileUploader.responseText || "").trim();
        if (detail !== "") {
            try {
                let res = JSON.parse(detail);
                hint = res && res.message ? res.message : detail;
            } catch (e) {
                hint = detail;
            }
        }
        imageUploadStatus.innerText = `文件上传失败：${hint}`;
    }, false);
    fileUploader.addEventListener("error", ev => {
        imageUploadStatus.innerText = "文件上传失败：网络错误或服务器无响应，请检查网络后重试";
        console.error(ev);
    }, false);
    fileUploader.addEventListener("abort", ev => {
        imageUploadStatus.innerText = "文件上传终止";
    }, false);
    fileUploader.addEventListener("readystatechange", ev => {
        // 只负责在成功时渲染图片链接面板，失败文案由 load 事件统一处理。
        if (fileUploader.readyState !== 4 || fileUploader.status !== 200) {
            return;
        }
        let res;
        try {
            res = JSON.parse(fileUploader.response);
        } catch (e) {
            console.error("image upload: invalid success response", e);
            return;
        }
        let filenames = res.data || [];
        let imageUploadPanel = document.getElementById('imageUploadPanel');
        filenames.forEach(filename => {
            let url = location.href + '/' + filename;
            imageUploadPanel.insertAdjacentHTML('afterbegin', `
        <div class="field has-addons">
            <div class="control is-light is-expanded">
                <input class="input url-input" type="text" value="${url}" readonly>
            </div>
            <div class="control">
                <a class="button is-light" onclick="copyText('${url}')">
                    复制链接
                </a>
            </div>
            <div class="control">
                <a class="button is-light" onclick="copyText('![${filename}](${url})')">
                    复制 Markdown 代码
                </a>
            </div>
        </div>
        `);
        });
    });
    fileUploader.open("POST", "/api/image");
    fileUploader.send(formData);
}

function imageDragOverHandler(ev) {
    ev.preventDefault();
}

function showMessage(message, isError = false) {
    const messageToast = document.getElementById('messageToast');
    messageToast.style.display = 'block';
    messageToast.className = isError ? "message is-danger" : "message";
    let timeout = isError ? 5000 : 2000;
    document.getElementById('messageToastText').innerText = message;
    if (isError) {
        document.getElementById("nav").scrollIntoView();
    }
    setTimeout(function () {
        messageToast.style.display = 'none';
    }, timeout);
}

function showQRCode(link) {
    let url = window.location.origin + link;
    url = encodeURI(url)
    console.log(url)
    let qr = new QRious({
        element: document.getElementById('qrcode'),
        value: url,
        size: 200,
    });
    showModal('qrcodeModal');
}

function copyLink(link) {
    let url = window.location.origin + link;
    url = decodeURI(url);
    copyText(url);
    showToast(`已复制：${url}`, 'success');
}

function toLocalTime(str) {
    let date = Date.parse(str);
    return date.toLocaleString()
}

function copyText(text) {
    const textArea = document.getElementById('hiddenTextArea');
    textArea.textContent = text;
    document.body.append(textArea);
    textArea.select();
    document.execCommand("copy");
}

function showToast(message, type = "success", timeout = 2900) {
    let toast = document.getElementById("toast");
    toast.innerText = message;
    toast.className = `show notification is-${type}`;
    setTimeout(() => {
        toast.className = "";
    }, timeout);
}

function showGeneralModal(title, content) {
    document.getElementById("generalModalTitle").innerText = title;
    document.getElementById("generalModalContent").innerHTML = content;
    showModal("generalModal");
}

// 统一的 JSON 接口请求封装：捕获网络错误与非 JSON 响应（如服务器 500 错误页），
// 避免操作后“无任何反馈”导致用户无法定位问题。业务层面的失败仍由调用方读取
// 返回的 success / message 字段处理。
async function requestJSON(url, options) {
    let response;
    try {
        response = await fetch(url, options);
    } catch (e) {
        throw new Error("网络错误或服务器无响应，请检查网络后重试");
    }
    let text = await response.text();
    try {
        return JSON.parse(text);
    } catch (e) {
        let detail = (text || "").trim();
        throw new Error(detail !== "" ? `服务器异常：${detail}` : `服务器返回状态码 ${response.status}，且无有效响应`);
    }
}

async function loadOptions() {
    let tab = document.getElementById('settingTab');
    let html = ""
    let result;
    try {
        result = await requestJSON("/api/option");
    } catch (e) {
        tab.innerHTML = `<p class="has-text-danger">选项加载失败：${e.message}</p>`;
        return;
    }
    if (result.success) {
        for (let i = 0; i < result.data.length; i++) {
            let key = result.data[i].key;
            let value = result.data[i].value;
            html += `
            <div>
                <label class="label">${key}</label>
                <div class="field has-addons">
                    <p class="control is-expanded">
                        <input class="input" id="inputOption${key}" type="text" placeholder="请输入新的配置" value="${value}">
                    </p>
                    <p class="control">
                        <a class="button" onclick="updateOption('${key}', 'inputOption${key}')">提交</a>
                    </p>
                </div>
            </div>`;
        }
    } else {
        html = `<p>选项加载失败：${result.message}</p>`
    }
    tab.innerHTML = html;
}

async function updateOption(key, inputElementId, originValue = "") {
    let inputElement = document.getElementById(inputElementId);
    let value = inputElement.value;
    let result;
    try {
        result = await requestJSON("/api/option", {
            method: "PUT",
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                key: key,
                value: value
            })
        });
    } catch (e) {
        showToast(`更新失败：${e.message}`, "danger");
        return;
    }
    if (result.success) {
        showToast(`更新成功`, "success");
    } else {
        showToast(`更新失败：${result.message}`, "danger");
        if (originValue !== "") {
            inputElement.value = originValue;
        }
    }
}

async function updateUser(key, inputElementId) {
    let inputElement = document.getElementById(inputElementId);
    let value = inputElement.value;
    if (value === "") return
    let data = {};
    data[key] = value;
    let result;
    try {
        result = await requestJSON("/api/user", {
            method: "PUT",
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    } catch (e) {
        showToast(`更新信息失败：${e.message}`, "danger");
        return;
    }
    if (result.success) {
        showToast(`更新信息成功`, "success");
    } else {
        showToast(`更新信息失败：${result.message}`, "danger");
    }
}

async function createUser() {
    let username = document.getElementById("newUserName").value;
    let password = document.getElementById("newUserPassword").value;
    if (!username || !password) return;
    let type = document.getElementById("newUserType").value;
    let data = {
        username: username,
        password: password,
        type: type
    }
    let result;
    try {
        result = await requestJSON("/api/user", {
            method: "POST",
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    } catch (e) {
        showToast(`添加用户失败：${e.message}`, "danger");
        return;
    }
    if (result.success) {
        showToast(`添加用户成功`, "success");
    } else {
        showToast(`添加用户失败：${result.message}`, "danger");
    }
}

async function manageUser() {
    let username = document.getElementById("manageUserName").value;
    let action = document.getElementById("manageAction").value;
    if (!username) return;

    let data = {
        username: username,
        action: action,
    }
    let result;
    try {
        result = await requestJSON("/api/manage_user", {
            method: "PUT",
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(data)
        });
    } catch (e) {
        showToast(`操作失败：${e.message}`, "danger");
        return;
    }
    if (result.success) {
        showToast(`操作成功`, "success");
    } else {
        showToast(`操作失败：${result.message}`, "danger");
    }
}

async function generateNewToken() {
    let result;
    try {
        result = await requestJSON("/api/token", {
            method: "POST",
            headers: {
                'Content-Type': 'application/json'
            }
        });
    } catch (e) {
        showToast(`操作失败：${e.message}`, "danger");
        return;
    }
    if (result.success) {
        showToast(`Token 已重置为：${result.data}`, "success");
    } else {
        showToast(`操作失败：${result.message}`, "danger");
    }
}

function isMobile() {
    return window.innerWidth <= 768;
}

function getFileExt(link) {
    let parts = link.split('.');
    if (parts.length === 1) return "";
    return parts[parts.length - 1].toLowerCase();
}

function getFilename(link) {
    let parts = link.split('/');
    return parts[parts.length - 1];
}

function displayFile(link) {
    // TODO: text file preview support
    let ext = getFileExt(link);
    let filename = getFilename(link);
    console.log(link, ext, filename)
    document.getElementById("displayModalTitle").innerText = filename;
    if (ext === "mp3" || ext === "wav" || ext === "ogg") {
        document.getElementById("displayModalContent").innerHTML = `
        <audio controls>
            <source src="${link}" type="audio/${ext}">
        </audio>`;
    } else if (ext === "mp4" || ext === "webm" || ext === "ogv") {
        document.getElementById("displayModalContent").innerHTML = `
        <video controls style="width: 100%">
            <source src="${link}" type="video/${ext}">
        </video>`;
    } else if (ext === "png" || ext === "jpg" || ext === "jpeg" || ext === "gif") {
        document.getElementById("displayModalContent").innerHTML = `
        <img src="${link}" alt="${filename}" width="100%">`;
    } else if (ext === "pdf") {
        if (isMobile()) {
            window.open(link);
            return;
        }
        document.getElementById("displayModalContent").innerHTML = `
        <div style="width:100%; height: 600px!important;">
            <iframe src="${link}" width="100%" height="100%"></iframe>
        </div>`;
    } else {
        window.open(link);
        return;
    }
    showModal("displayModal");
}

function init() {
    const $navbarBurgers = Array.prototype.slice.call(document.querySelectorAll('.navbar-burger'), 0);
    if ($navbarBurgers.length > 0) {
        $navbarBurgers.forEach(el => {
            el.addEventListener('click', () => {
                const target = el.dataset.target;
                const $target = document.getElementById(target);
                el.classList.toggle('is-active');
                $target.classList.toggle('is-active');
            });
        });
    }

    hiddenTextArea = document.createElement('textarea');
    hiddenTextArea.setAttribute("id", "hiddenTextArea");
    hiddenTextArea.style.cssText = "height: 0px; width: 0px";
    document.body.appendChild(hiddenTextArea);
}

document.addEventListener('DOMContentLoaded', init)