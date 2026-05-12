document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('standalone-login-form');
  const username = document.getElementById('login-username');
  const password = document.getElementById('login-password');
  const errorBox = document.getElementById('login-error');

  checkExistingSession();

  form.addEventListener('submit', async event => {
    event.preventDefault();
    errorBox.textContent = '';

    try {
      const response = await fetch('/api/login', {
        method: 'POST',
        cache: 'no-store',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          username: username.value.trim(),
          password: password.value
        })
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok || data.error) {
        throw new Error(data.error || '登录失败');
      }
      password.value = '';
      window.location.replace('/');
    } catch (error) {
      errorBox.textContent = error.message || '登录失败';
    }
  });
});

async function checkExistingSession() {
  try {
    const response = await fetch('/api/auth', {
      cache: 'no-store',
      headers: { Accept: 'application/json' }
    });
    const data = await response.json();
    if (!data.enabled || data.authenticated) {
      window.location.replace('/');
    }
  } catch (error) {
    document.getElementById('login-error').textContent = `认证状态读取失败：${error.message}`;
  }
}
