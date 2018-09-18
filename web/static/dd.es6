//

window.authToken = localStorage.getItem('auth-token');

$('#register').click(function () {
    window.authAction([
        'register', $('#uid').val(), $('#pwd').val(),
    ]);
    window.authAction = null;
});

$('#login').click(function () {
    window.authAction([
        'login', $('#uid').val(), $('#pwd').val(),
    ]);
    window.authAction = null;
});

async function getAuthToken(tid) {

    let uid = localStorage.getItem('user-id');
    let pwd = '';
    let tkn = window.authToken;

    while (true) {

        if (uid && (pwd || tkn)) {
            let result = await $.ajax({
                dataType: 'json', method: 'post', url: '/api/' + tid + '/auth',
                contentType: "application/json", data: JSON.stringify({
                    uid: uid, pwd: pwd, tkn: tkn,
                }),
            });
            tkn = result.tkn;
            if (result.err) {
                $('#authMsg').html(result.err);
            }
        }

        if (tkn) {
            window.authToken = tkn;
            localStorage.setItem('user-id', uid);
            localStorage.setItem('auth-token', tkn);

            $('#login-background,#login-dialog').removeClass('active');

            return tkn;
        }

        $('#login-background,#login-dialog').addClass('active');

        while (true) {
            let act;
            [act, uid, pwd] = await new Promise((resolve, reject) => {
                window.authAction = resolve;
            });

            if ('login' === act) {
                break;
            }

            // assume register
            let result = await $.ajax({
                dataType: 'json', method: 'post', url: '/api/' + tid + '/register',
                contentType: "application/json", data: JSON.stringify({
                    uid: uid, pwd: pwd,
                }),
            });
            if (result.err) {
                // register failed
                $('#authMsg').html(result.err);
                continue
            }
            // register okay
            break;
        }

    }

}
